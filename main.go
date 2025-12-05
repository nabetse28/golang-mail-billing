package main

import (
	"context"
	"fmt"
	"time"

	"github.com/nabetse28/golang-mail-billing/config"
	"github.com/nabetse28/golang-mail-billing/gmailclient"
	"github.com/nabetse28/golang-mail-billing/logging"
	"github.com/nabetse28/golang-mail-billing/storage"

	gmail "google.golang.org/api/gmail/v1"
)

// ------------------------------------------------------------
// Worker job structure
// ------------------------------------------------------------
type EmailJob struct {
	MessageID string
}

// ------------------------------------------------------------
// Start worker pool
// ------------------------------------------------------------
func startWorkers(n int, workerFunc func(job EmailJob)) chan<- EmailJob {
	jobs := make(chan EmailJob, n*5) // small buffer

	for i := 0; i < n; i++ {
		go func(id int) {
			logging.Infof("Worker %d started", id)
			for job := range jobs {
				workerFunc(job)
			}
		}(i + 1)
	}

	return jobs
}

// ------------------------------------------------------------
// Main email processor (executed by workers)
// ------------------------------------------------------------
func processEmail(
	messageID string,
	srv *gmail.Service,
	user string,
	cfg *config.Config,
	labelService *gmailclient.LabelService,
	processedLabelID string,
) {
	// ------------------------------------------------------------
	// 1) Fetch metadata (lightweight)
	// ------------------------------------------------------------
	fullMsg, err := srv.Users.Messages.Get(user, messageID).Format("metadata").Do()
	if err != nil {
		logging.Errorf("Cannot fetch message metadata %s: %v", messageID, err)
		return
	}

	alreadyProcessed := contains(fullMsg.LabelIds, processedLabelID)

	// ------------------------------------------------------------
	// 2) FORCE REPROCESS ALWAYS overrides everything
	// ------------------------------------------------------------
	if alreadyProcessed && cfg.Gmail.ForceReprocess {
		logging.Infof("Force reprocess enabled. Reprocessing %s...", messageID)
		// Skip to main processing logic
		goto PROCESS_MESSAGE
	}

	// ------------------------------------------------------------
	// 3) If processed before → verify if all attachments exist locally
	// ------------------------------------------------------------
	if alreadyProcessed {
		logging.Infof("Message %s is marked as processed. Verifying local attachments...", messageID)

		msgDate, err := gmailclient.GetMessageDateOnly(srv, user, messageID)
		if err != nil {
			logging.Errorf("Failed to get date for %s: %v", messageID, err)
			return
		}

		dir, err := storage.EnsureInvoiceDir(
			cfg.Paths.BaseInvoicesPath,
			msgDate.Year(),
			int(msgDate.Month()),
		)
		if err != nil {
			logging.Errorf("Failed to ensure dir for %s: %v", messageID, err)
			return
		}

		// fetch attachment names from Gmail headers
		expectedFiles, err := gmailclient.GetAttachmentNames(srv, user, messageID)
		if err != nil {
			logging.Errorf("Failed reading attachment names for %s: %v", messageID, err)
			return
		}

		// check for missing files
		missing := false
		for _, f := range expectedFiles {
			if !storage.FileExists(dir, f) {
				logging.Infof("Missing attachment %s in %s → must reprocess", f, dir)
				missing = true
			}
		}

		if !missing {
			logging.Infof("All local attachments exist → skipping %s", messageID)
			return
		}

		logging.Infof("Reprocessing %s because missing attachments were detected", messageID)
	}

PROCESS_MESSAGE:

	// ------------------------------------------------------------
	// 4) Get message date + organize labels
	// ------------------------------------------------------------
	var msgDate time.Time
	if cfg.Gmail.DownloadOnly {
		msgDate, err = gmailclient.GetMessageDateOnly(srv, user, messageID)
	} else {
		msgDate, err = gmailclient.OrganizeMessageByDate(
			srv,
			user,
			cfg.Gmail.BaseBillingLabel,
			labelService,
			messageID,
		)
	}

	if err != nil {
		logging.Errorf("Failed organizing or getting date for %s: %v", messageID, err)
		return
	}

	// ------------------------------------------------------------
	// 5) Apply year/month filters
	// ------------------------------------------------------------
	if (cfg.Gmail.FilterYear != 0 && msgDate.Year() != cfg.Gmail.FilterYear) ||
		(cfg.Gmail.FilterMonth != 0 && int(msgDate.Month()) != cfg.Gmail.FilterMonth) {

		logging.Infof("Skipping %s due to date filter mismatch", messageID)
		return
	}

	// ------------------------------------------------------------
	// 6) Ensure local directory
	// ------------------------------------------------------------
	dir, err := storage.EnsureInvoiceDir(
		cfg.Paths.BaseInvoicesPath,
		msgDate.Year(),
		int(msgDate.Month()),
	)
	if err != nil {
		logging.Errorf("Cannot ensure directory for %s: %v", messageID, err)
		return
	}

	// ------------------------------------------------------------
	// 7) Download attachments
	// ------------------------------------------------------------
	if err := gmailclient.DownloadAttachmentsToDir(srv, user, messageID, dir); err != nil {
		logging.Errorf("Failed downloading attachments for %s: %v", messageID, err)
		return
	}

	// ------------------------------------------------------------
	// 8) Mark processed
	// ------------------------------------------------------------
	if !cfg.Gmail.DownloadOnly {
		_, err := srv.Users.Messages.Modify(user, messageID, &gmail.ModifyMessageRequest{
			AddLabelIds: []string{processedLabelID},
		}).Do()

		if err != nil {
			logging.Errorf("Failed marking %s as processed: %v", messageID, err)
		} else {
			logging.Infof("Message %s marked as processed", messageID)
		}
	}
}


// ------------------------------------------------------------
// MAIN
// ------------------------------------------------------------
func main() {
	ctx := context.Background()
	logging.Init()

	logging.Infof("Starting Gmail organizer...")

	// ------------------------------------------------------------
	// 1) Load config
	// ------------------------------------------------------------
	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		logging.Fatalf("Failed to load config: %v", err)
	}

	logging.Infof(
		"Config loaded. user=%s query=%s max_results=%d base_label=%s invoices_path=%s filter_year=%d filter_month=%d download_only=%t",
		cfg.Gmail.User, cfg.Gmail.Query, cfg.Gmail.MaxResults,
		cfg.Gmail.BaseBillingLabel, cfg.Paths.BaseInvoicesPath,
		cfg.Gmail.FilterYear, cfg.Gmail.FilterMonth, cfg.Gmail.DownloadOnly,
	)

	// ------------------------------------------------------------
	// 2) Gmail service
	// ------------------------------------------------------------
	srv, err := gmailclient.NewService(ctx, "credentials.json", gmail.GmailModifyScope)
	if err != nil {
		logging.Fatalf("Failed to create Gmail service: %v", err)
	}

	user := cfg.Gmail.User

	// ------------------------------------------------------------
	// 3) Label service
	// ------------------------------------------------------------
	var labelService *gmailclient.LabelService

	if !cfg.Gmail.DownloadOnly {
		labelService, err = gmailclient.NewLabelService(srv, user)
		if err != nil {
			logging.Fatalf("Failed to init LabelService: %v", err)
		}
	}

	processedLabelName := fmt.Sprintf("%s/Processed", cfg.Gmail.BaseBillingLabel)
	processedLabelID := ""

	if !cfg.Gmail.DownloadOnly {
		processedLabelID, err = labelService.EnsureLabel(processedLabelName)
		if err != nil {
			logging.Fatalf("Cannot ensure processed label: %v", err)
		}
		logging.Infof("Using Processed label: %s (ID=%s)", processedLabelName, processedLabelID)
	}

	// ------------------------------------------------------------
	// 4) Start Worker Pool
	// ------------------------------------------------------------
	workerCount := 4
	jobs := startWorkers(workerCount, func(job EmailJob) {
		processEmail(job.MessageID, srv, user, cfg, labelService, processedLabelID)
	})

	// ------------------------------------------------------------
	// 5) Pagination Loop
	// ------------------------------------------------------------
	pageToken := ""
	pageNum := 0
	totalMsgs := 0

	for {
		pageNum++
		logging.Infof("Requesting page %d (token=%q)...", pageNum, pageToken)

		call := srv.Users.Messages.List(user).
			Q(cfg.Gmail.Query).
			MaxResults(cfg.Gmail.MaxResults)

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			logging.Fatalf("Failed listing messages: %v", err)
		}

		if len(resp.Messages) == 0 {
			logging.Infof("Page %d returned 0 messages.", pageNum)
			break
		}

		logging.Infof("Processing %d messages on page %d...", len(resp.Messages), pageNum)

		for _, m := range resp.Messages {
			totalMsgs++
			jobs <- EmailJob{MessageID: m.Id}
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	close(jobs)

	logging.Infof("Finished. Total messages queued = %d.", totalMsgs)
}

// ------------------------------------------------------------
// Helpers
// ------------------------------------------------------------
func contains(list []string, value string) bool {
	for _, x := range list {
		if x == value {
			return true
		}
	}
	return false
}
