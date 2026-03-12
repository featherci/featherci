package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/featherci/featherci/internal/crypto"
	"github.com/featherci/featherci/internal/executor"
	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/notify"
)

// NotificationService provides business logic for notification channels.
type NotificationService struct {
	channels     models.NotificationChannelRepository
	steps        models.BuildStepRepository
	encryptor    *crypto.Encryptor
	baseURL      string
	devMode      bool
	previewStore *notify.PreviewStore
	logger       *slog.Logger
}

// NewNotificationService creates a new NotificationService.
func NewNotificationService(
	channels models.NotificationChannelRepository,
	steps models.BuildStepRepository,
	encryptor *crypto.Encryptor,
	baseURL string,
	devMode bool,
	logger *slog.Logger,
) *NotificationService {
	svc := &NotificationService{
		channels:  channels,
		steps:     steps,
		encryptor: encryptor,
		baseURL:   baseURL,
		devMode:   devMode,
		logger:    logger,
	}
	if devMode {
		svc.previewStore = notify.NewPreviewStore()
	}
	return svc
}

// PreviewStore returns the dev-mode preview store, or nil if not in dev mode.
func (s *NotificationService) PreviewStore() *notify.PreviewStore {
	return s.previewStore
}

// CreateChannel validates, encrypts config, and creates a notification channel.
func (s *NotificationService) CreateChannel(ctx context.Context, projectID int64, name, channelType string, configJSON map[string]string, onSuccess, onFailure, onCancelled bool, userID int64) error {
	if name == "" {
		return fmt.Errorf("channel name is required")
	}

	// Validate that the channel type is known and config is valid by trying to create a notifier
	if _, err := notify.NewNotifier(channelType, configJSON); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	configBytes, err := json.Marshal(configJSON)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	encrypted, err := s.encryptor.Encrypt(configBytes)
	if err != nil {
		return fmt.Errorf("encrypting config: %w", err)
	}

	channel := &models.NotificationChannel{
		ProjectID:       projectID,
		Name:            name,
		Type:            channelType,
		ConfigEncrypted: encrypted,
		OnSuccess:       onSuccess,
		OnFailure:       onFailure,
		OnCancelled:     onCancelled,
		Enabled:         true,
		CreatedBy:       userID,
	}

	return s.channels.Create(ctx, channel)
}

// UpdateChannel updates a notification channel's settings and config.
func (s *NotificationService) UpdateChannel(ctx context.Context, channelID int64, name string, configJSON map[string]string, onSuccess, onFailure, onCancelled bool) error {
	channel, err := s.channels.GetByID(ctx, channelID)
	if err != nil {
		return fmt.Errorf("getting channel: %w", err)
	}

	if name == "" {
		return fmt.Errorf("channel name is required")
	}

	// Validate config
	if _, err := notify.NewNotifier(channel.Type, configJSON); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	configBytes, err := json.Marshal(configJSON)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	encrypted, err := s.encryptor.Encrypt(configBytes)
	if err != nil {
		return fmt.Errorf("encrypting config: %w", err)
	}

	channel.Name = name
	channel.ConfigEncrypted = encrypted
	channel.OnSuccess = onSuccess
	channel.OnFailure = onFailure
	channel.OnCancelled = onCancelled

	return s.channels.Update(ctx, channel)
}

// DeleteChannel removes a notification channel.
func (s *NotificationService) DeleteChannel(ctx context.Context, channelID int64) error {
	return s.channels.Delete(ctx, channelID)
}

// ListChannels returns all channels for a project (metadata only, no decrypted config).
func (s *NotificationService) ListChannels(ctx context.Context, projectID int64) ([]*models.NotificationChannel, error) {
	return s.channels.ListByProject(ctx, projectID)
}

// GetChannel returns a channel with its decrypted config for editing.
func (s *NotificationService) GetChannel(ctx context.Context, channelID int64) (*models.NotificationChannel, map[string]string, error) {
	channel, err := s.channels.GetByID(ctx, channelID)
	if err != nil {
		return nil, nil, err
	}

	configBytes, err := s.encryptor.Decrypt(channel.ConfigEncrypted)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypting config: %w", err)
	}

	var config map[string]string
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return nil, nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	return channel, config, nil
}

// TestChannel sends a test notification through a channel.
// In dev mode, it captures three examples (success, failure, cancelled) for preview.
func (s *NotificationService) TestChannel(ctx context.Context, channelID int64) error {
	channel, config, err := s.GetChannel(ctx, channelID)
	if err != nil {
		return err
	}

	base := notify.BuildEvent{
		ProjectName:   "test-project",
		BuildNumber:   42,
		Branch:        "main",
		CommitSHA:     "abc1234def5678",
		CommitMessage: "Add user authentication flow",
		CommitAuthor:  "FeatherCI",
		Duration:      2*time.Minute + 35*time.Second,
		BuildURL:      s.baseURL,
		ProjectURL:    s.baseURL,
		CommitURL:     "https://github.com/example/test-project/commit/abc1234def5678",
	}

	if s.devMode && s.previewStore != nil {
		// Capture success example
		success := base
		success.Status = "success"
		s.previewStore.Capture(channel.Name, channel.Type, success, config["from"], config["to"])

		// Capture failure example with failed steps and log snippets
		failure := base
		failure.Status = "failure"
		failure.BuildNumber = 43
		failure.FailedSteps = []notify.FailedStep{
			{
				Name: "minitest",
				LogLines: []string{
					"Failure:",
					"UserAuthTest#test_login_with_invalid_credentials [test/models/user_auth_test.rb:42]:",
					"Expected: \"Invalid email or password\"",
					`  Actual: nil`,
					"rails test test/models/user_auth_test.rb:38",
				},
			},
			{
				Name: "system",
				LogLines: []string{
					"Error:",
					"ActionView::Template::Error: undefined method `full_name' for nil",
					"    app/views/profiles/show.html.erb:12",
					"    test/system/profiles_test.rb:28:in `test_view_profile'",
					"1 runs, 0 assertions, 0 failures, 1 errors, 0 skips",
				},
			},
		}
		s.previewStore.Capture(channel.Name, channel.Type, failure, config["from"], config["to"])

		// Capture cancelled example
		cancelled := base
		cancelled.Status = "cancelled"
		cancelled.BuildNumber = 44
		cancelled.Duration = 18 * time.Second
		s.previewStore.Capture(channel.Name, channel.Type, cancelled, config["from"], config["to"])

		return nil
	}

	// Production: send only the success test event
	event := base
	event.Status = "success"
	notifier, err := notify.NewNotifier(channel.Type, config)
	if err != nil {
		return fmt.Errorf("creating notifier: %w", err)
	}

	return notifier.Send(ctx, event)
}

// NotifyBuild sends notifications for a completed build to all matching channels.
func (s *NotificationService) NotifyBuild(ctx context.Context, build *models.Build, project *models.Project) error {
	channels, err := s.channels.ListByProject(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("listing channels: %w", err)
	}

	isSuccess := build.Status == models.BuildStatusSuccess
	isCancelled := build.Status == models.BuildStatusCancelled
	branch := ""
	if build.Branch != nil {
		branch = *build.Branch
	}
	commitMsg := ""
	if build.CommitMessage != nil {
		commitMsg = *build.CommitMessage
	}
	commitAuthor := ""
	if build.CommitAuthor != nil {
		commitAuthor = *build.CommitAuthor
	}

	projectURL := fmt.Sprintf("%s/projects/%s/%s", s.baseURL, project.Namespace, project.Name)

	// For failed builds, collect failed steps with log tails
	var failedSteps []notify.FailedStep
	if build.Status == models.BuildStatusFailure && s.steps != nil {
		failedSteps = s.loadFailedSteps(ctx, build.ID)
	}

	event := notify.BuildEvent{
		ProjectName:   project.FullName,
		BuildNumber:   build.BuildNumber,
		Status:        string(build.Status),
		Branch:        branch,
		CommitSHA:     build.CommitSHA,
		CommitMessage: commitMsg,
		CommitAuthor:  commitAuthor,
		Duration:      build.Duration(),
		BuildURL:      fmt.Sprintf("%s/builds/%d", projectURL, build.BuildNumber),
		ProjectURL:    projectURL,
		CommitURL:     commitURL(project.CloneURL, project.Provider, build.CommitSHA),
		FailedSteps:   failedSteps,
	}

	var wg sync.WaitGroup
	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		if isSuccess && !ch.OnSuccess {
			continue
		}
		if isCancelled && !ch.OnCancelled {
			continue
		}
		if !isSuccess && !isCancelled && !ch.OnFailure {
			continue
		}

		// In dev mode, capture instead of sending
		if s.devMode && s.previewStore != nil {
			from, to := "", ""
			if configBytes, err := s.encryptor.Decrypt(ch.ConfigEncrypted); err == nil {
				var cfg map[string]string
				if err := json.Unmarshal(configBytes, &cfg); err == nil {
					from, to = cfg["from"], cfg["to"]
				}
			}
			s.previewStore.Capture(ch.Name, ch.Type, event, from, to)
			continue
		}

		wg.Add(1)
		go func(channel *models.NotificationChannel) {
			defer wg.Done()

			configBytes, err := s.encryptor.Decrypt(channel.ConfigEncrypted)
			if err != nil {
				s.logger.Error("failed to decrypt notification config",
					"channel_id", channel.ID, "error", err)
				return
			}

			var config map[string]string
			if err := json.Unmarshal(configBytes, &config); err != nil {
				s.logger.Error("failed to unmarshal notification config",
					"channel_id", channel.ID, "error", err)
				return
			}

			notifier, err := notify.NewNotifier(channel.Type, config)
			if err != nil {
				s.logger.Error("failed to create notifier",
					"channel_id", channel.ID, "type", channel.Type, "error", err)
				return
			}

			if err := notifier.Send(ctx, event); err != nil {
				s.logger.Error("failed to send notification",
					"channel_id", channel.ID, "type", channel.Type, "error", err)
			}
		}(ch)
	}
	wg.Wait()
	return nil
}

// commitURL derives the web URL to view a commit on the git provider.
func commitURL(cloneURL, provider, sha string) string {
	u, err := url.Parse(cloneURL)
	if err != nil {
		return ""
	}
	// Strip .git suffix and credentials
	path := strings.TrimSuffix(u.Path, ".git")
	u.User = nil
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	base := u.String()

	switch provider {
	case "gitlab":
		return base + path + "/-/commit/" + sha
	default: // github, gitea/forgejo
		return base + path + "/commit/" + sha
	}
}

const failedStepLogLines = 5

// loadFailedSteps retrieves failed steps for a build with their last log lines.
func (s *NotificationService) loadFailedSteps(ctx context.Context, buildID int64) []notify.FailedStep {
	steps, err := s.steps.ListByBuild(ctx, buildID)
	if err != nil {
		s.logger.Error("failed to load steps for notification", "build_id", buildID, "error", err)
		return nil
	}

	var failed []notify.FailedStep
	for _, step := range steps {
		if step.Status != models.StepStatusFailure {
			continue
		}
		fs := notify.FailedStep{Name: step.Name}
		if step.LogPath != nil && *step.LogPath != "" {
			total, err := executor.CountLines(*step.LogPath)
			if err == nil && total > 0 {
				offset := total - failedStepLogLines
				if offset < 0 {
					offset = 0
				}
				lines, err := executor.ReadLines(*step.LogPath, offset, failedStepLogLines)
				if err == nil {
					fs.LogLines = lines
				}
			}
		}
		failed = append(failed, fs)
	}
	return failed
}
