package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/featherci/featherci/internal/middleware"
	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/notify"
	"github.com/featherci/featherci/internal/services"
	"github.com/featherci/featherci/internal/templates"
)

// NotificationListPageData holds data for the notification channels list page.
type NotificationListPageData struct {
	User     *models.User
	Project  *models.Project
	Channels []*models.NotificationChannel
	Success  string
	Error    string
	DevMode  bool
}

// NotificationFormPageData holds data for the add/edit notification form.
type NotificationFormPageData struct {
	User         *models.User
	Project      *models.Project
	Channel      *models.NotificationChannel
	Config       map[string]string
	ChannelTypes []string
	Error        string
}

// NotificationPreviewPageData holds data for the dev-mode preview page.
type NotificationPreviewPageData struct {
	User    *models.User
	Entries []notify.PreviewEntry
}

// NotificationPreviewDetailData holds data for a single preview entry.
type NotificationPreviewDetailData struct {
	User  *models.User
	Entry notify.PreviewEntry
}

// NotificationHandler handles notification channel HTTP requests.
type NotificationHandler struct {
	notifications *services.NotificationService
	projects      models.ProjectRepository
	projectUsers  models.ProjectUserRepository
	templates     *templates.Engine
	logger        *slog.Logger
	devMode       bool
}

// NewNotificationHandler creates a new NotificationHandler.
func NewNotificationHandler(
	notifications *services.NotificationService,
	projects models.ProjectRepository,
	projectUsers models.ProjectUserRepository,
	tmpl *templates.Engine,
	logger *slog.Logger,
	devMode bool,
) *NotificationHandler {
	return &NotificationHandler{
		notifications: notifications,
		projects:      projects,
		projectUsers:  projectUsers,
		templates:     tmpl,
		logger:        logger,
		devMode:       devMode,
	}
}

// lookupProject finds a project by namespace/name, trying all providers.
func (h *NotificationHandler) lookupProject(r *http.Request) (*models.Project, error) {
	ctx := r.Context()
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")
	fullName := namespace + "/" + name

	var project *models.Project
	var lastErr error
	for _, provider := range []string{"github", "gitlab", "gitea"} {
		p, err := h.projects.GetByFullName(ctx, provider, fullName)
		if err == nil {
			project = p
			break
		}
		lastErr = err
		if !errors.Is(err, models.ErrNotFound) {
			return nil, err
		}
	}
	if project == nil {
		return nil, lastErr
	}
	return project, nil
}

// requireManage checks auth and project manage permission.
func (h *NotificationHandler) requireManage(w http.ResponseWriter, r *http.Request) (*models.User, *models.Project, bool) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return nil, nil, false
	}

	project, err := h.lookupProject(r)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return nil, nil, false
		}
		h.logger.Error("failed to get project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return nil, nil, false
	}

	canManage, err := h.projectUsers.CanUserManage(r.Context(), project.ID, user.ID)
	if err != nil {
		h.logger.Error("failed to check manage permission", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return nil, nil, false
	}
	if !canManage && !user.IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return nil, nil, false
	}

	return user, project, true
}

// List shows all notification channels for a project.
// GET /projects/{namespace}/{name}/notifications
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	user, project, ok := h.requireManage(w, r)
	if !ok {
		return
	}

	channels, err := h.notifications.ListChannels(r.Context(), project.ID)
	if err != nil {
		h.logger.Error("failed to list notification channels", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := NotificationListPageData{
		User:     user,
		Project:  project,
		Channels: channels,
		Success:  r.URL.Query().Get("success"),
		Error:    r.URL.Query().Get("error"),
		DevMode:  h.devMode,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.Render(w, "pages/notifications/list.html", data); err != nil {
		h.logger.Error("failed to render notification list", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// New shows the form to add a new notification channel.
// GET /projects/{namespace}/{name}/notifications/new
func (h *NotificationHandler) New(w http.ResponseWriter, r *http.Request) {
	user, project, ok := h.requireManage(w, r)
	if !ok {
		return
	}

	data := NotificationFormPageData{
		User:         user,
		Project:      project,
		ChannelTypes: notify.ChannelTypes(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.Render(w, "pages/notifications/form.html", data); err != nil {
		h.logger.Error("failed to render notification form", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Create adds a new notification channel.
// POST /projects/{namespace}/{name}/notifications
func (h *NotificationHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, project, ok := h.requireManage(w, r)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	redirectURL := fmt.Sprintf("/projects/%s/%s/notifications", project.Namespace, project.Name)

	name := r.FormValue("name")
	channelType := r.FormValue("type")
	onSuccess := r.FormValue("on_success") == "on"
	onFailure := r.FormValue("on_failure") == "on"
	onCancelled := r.FormValue("on_cancelled") == "on"

	config := extractConfig(r, channelType)

	if err := h.notifications.CreateChannel(r.Context(), project.ID, name, channelType, config, onSuccess, onFailure, onCancelled, user.ID); err != nil {
		http.Redirect(w, r, redirectURL+"?error="+err.Error(), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, redirectURL+"?success=Notification+channel+created", http.StatusSeeOther)
}

// Edit shows the form to edit an existing notification channel.
// GET /projects/{namespace}/{name}/notifications/{id}/edit
func (h *NotificationHandler) Edit(w http.ResponseWriter, r *http.Request) {
	user, project, ok := h.requireManage(w, r)
	if !ok {
		return
	}

	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	channel, config, err := h.notifications.GetChannel(r.Context(), channelID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get notification channel", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Verify channel belongs to this project
	if channel.ProjectID != project.ID {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	data := NotificationFormPageData{
		User:         user,
		Project:      project,
		Channel:      channel,
		Config:       config,
		ChannelTypes: notify.ChannelTypes(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.Render(w, "pages/notifications/form.html", data); err != nil {
		h.logger.Error("failed to render notification form", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Update saves changes to a notification channel.
// POST /projects/{namespace}/{name}/notifications/{id}
func (h *NotificationHandler) Update(w http.ResponseWriter, r *http.Request) {
	_, project, ok := h.requireManage(w, r)
	if !ok {
		return
	}

	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	redirectURL := fmt.Sprintf("/projects/%s/%s/notifications", project.Namespace, project.Name)

	// Verify channel belongs to this project
	channel, _, err := h.notifications.GetChannel(r.Context(), channelID)
	if err != nil {
		http.Redirect(w, r, redirectURL+"?error=Channel+not+found", http.StatusSeeOther)
		return
	}
	if channel.ProjectID != project.ID {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	name := r.FormValue("name")
	onSuccess := r.FormValue("on_success") == "on"
	onFailure := r.FormValue("on_failure") == "on"
	onCancelled := r.FormValue("on_cancelled") == "on"

	config := extractConfig(r, channel.Type)

	if err := h.notifications.UpdateChannel(r.Context(), channelID, name, config, onSuccess, onFailure, onCancelled); err != nil {
		http.Redirect(w, r, redirectURL+"?error="+err.Error(), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, redirectURL+"?success=Notification+channel+updated", http.StatusSeeOther)
}

// Delete removes a notification channel.
// POST /projects/{namespace}/{name}/notifications/{id}/delete
func (h *NotificationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	_, project, ok := h.requireManage(w, r)
	if !ok {
		return
	}

	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	redirectURL := fmt.Sprintf("/projects/%s/%s/notifications", project.Namespace, project.Name)

	// Verify channel belongs to this project
	channel, _, err := h.notifications.GetChannel(r.Context(), channelID)
	if err != nil {
		http.Redirect(w, r, redirectURL+"?error=Channel+not+found", http.StatusSeeOther)
		return
	}
	if channel.ProjectID != project.ID {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	if err := h.notifications.DeleteChannel(r.Context(), channelID); err != nil {
		h.logger.Error("failed to delete notification channel", "error", err)
		http.Redirect(w, r, redirectURL+"?error=Failed+to+delete+channel", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, redirectURL+"?success=Notification+channel+deleted", http.StatusSeeOther)
}

// Test sends a test notification through a channel.
// POST /projects/{namespace}/{name}/notifications/{id}/test
func (h *NotificationHandler) Test(w http.ResponseWriter, r *http.Request) {
	_, project, ok := h.requireManage(w, r)
	if !ok {
		return
	}

	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	redirectURL := fmt.Sprintf("/projects/%s/%s/notifications", project.Namespace, project.Name)

	// Verify channel belongs to this project
	channel, _, err := h.notifications.GetChannel(r.Context(), channelID)
	if err != nil {
		http.Redirect(w, r, redirectURL+"?error=Channel+not+found", http.StatusSeeOther)
		return
	}
	if channel.ProjectID != project.ID {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	if err := h.notifications.TestChannel(r.Context(), channelID); err != nil {
		http.Redirect(w, r, redirectURL+"?error=Test+failed:+"+err.Error(), http.StatusSeeOther)
		return
	}

	msg := "Test+notification+sent"
	if h.devMode {
		msg = "Test+notification+captured+(check+preview)"
	}
	http.Redirect(w, r, redirectURL+"?success="+msg, http.StatusSeeOther)
}

// PreviewList shows captured notifications in dev mode.
// GET /notifications/preview
func (h *NotificationHandler) PreviewList(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	store := h.notifications.PreviewStore()
	if store == nil {
		http.Error(w, "Preview only available in dev mode", http.StatusNotFound)
		return
	}

	data := NotificationPreviewPageData{
		User:    user,
		Entries: store.List(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.Render(w, "pages/notifications/preview.html", data); err != nil {
		h.logger.Error("failed to render notification preview", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// PreviewShow shows a single captured notification in dev mode.
// GET /notifications/preview/{id}
func (h *NotificationHandler) PreviewShow(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	store := h.notifications.PreviewStore()
	if store == nil {
		http.Error(w, "Preview only available in dev mode", http.StatusNotFound)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	entry, found := store.Get(id)
	if !found {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	data := NotificationPreviewDetailData{
		User:  user,
		Entry: entry,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.Render(w, "pages/notifications/preview_detail.html", data); err != nil {
		h.logger.Error("failed to render notification preview detail", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// PreviewRaw serves the raw rendered HTML for an email preview (used in iframe).
// GET /notifications/preview/{id}/raw
func (h *NotificationHandler) PreviewRaw(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	store := h.notifications.PreviewStore()
	if store == nil {
		http.Error(w, "Preview only available in dev mode", http.StatusNotFound)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	entry, found := store.Get(id)
	if !found {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if entry.HTML != "" {
		w.Write([]byte(entry.HTML))
	} else {
		w.Write([]byte("<p>No HTML preview available for this notification type.</p>"))
	}
}

// extractConfig pulls the config fields from the form based on channel type.
func extractConfig(r *http.Request, channelType string) map[string]string {
	config := make(map[string]string)
	switch channelType {
	case "email_smtp":
		for _, k := range []string{"host", "port", "username", "password", "from", "to"} {
			if v := r.FormValue("config_" + k); v != "" {
				config[k] = v
			}
		}
	case "email_sendgrid":
		for _, k := range []string{"api_key", "from", "to"} {
			if v := r.FormValue("config_" + k); v != "" {
				config[k] = v
			}
		}
	case "email_mailgun":
		for _, k := range []string{"api_key", "domain", "from", "to"} {
			if v := r.FormValue("config_" + k); v != "" {
				config[k] = v
			}
		}
	case "slack", "discord":
		if v := r.FormValue("config_webhook_url"); v != "" {
			config["webhook_url"] = v
		}
	case "pushover":
		for _, k := range []string{"app_token", "user_key"} {
			if v := r.FormValue("config_" + k); v != "" {
				config[k] = v
			}
		}
	}
	return config
}
