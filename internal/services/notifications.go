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
	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/notify"
)

// NotificationService provides business logic for notification channels.
type NotificationService struct {
	channels     models.NotificationChannelRepository
	encryptor    *crypto.Encryptor
	baseURL      string
	devMode      bool
	previewStore *notify.PreviewStore
	logger       *slog.Logger
}

// NewNotificationService creates a new NotificationService.
func NewNotificationService(
	channels models.NotificationChannelRepository,
	encryptor *crypto.Encryptor,
	baseURL string,
	devMode bool,
	logger *slog.Logger,
) *NotificationService {
	svc := &NotificationService{
		channels:  channels,
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
func (s *NotificationService) TestChannel(ctx context.Context, channelID int64) error {
	channel, config, err := s.GetChannel(ctx, channelID)
	if err != nil {
		return err
	}

	event := notify.BuildEvent{
		ProjectName:   "test-project",
		BuildNumber:   42,
		Status:        "success",
		Branch:        "main",
		CommitSHA:     "abc1234def5678",
		CommitMessage: "This is a test notification from FeatherCI",
		CommitAuthor:  "FeatherCI",
		Duration:      45 * time.Second,
		BuildURL:      s.baseURL,
		ProjectURL:    s.baseURL,
		CommitURL:     "https://github.com/example/test-project/commit/abc1234def5678",
	}

	if s.devMode && s.previewStore != nil {
		s.previewStore.Capture(channel.Name, channel.Type, event, config["from"], config["to"])
		return nil
	}

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
