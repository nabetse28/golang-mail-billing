package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
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

func printHelp() {
	fmt.Println(`
Gmail Billing Organizer
-----------------------
Automates labeling and downloading Gmail attachments for billing purposes.

USAGE:
  gmail-billing [flags]

FLAGS:
  -config <path>          Path to YAML configuration file (default: config/config.yaml)
  -query <gmail-query>    Override Gmail search query
  -filter-year <YYYY>     Only process emails from this year
  -filter-month <MM>      Only process emails from this month (1-12)
  -download-only          Do NOT modify Gmail labels, only download attachments
  -force-reprocess        Reprocess messages even if they have already been marked as processed
  -h, -help, --help       Show this help message

EXAMPLES:

  # Run with default config
  gmail-billing

  # Only download attachments from November 2025
  gmail-billing -filter-year 2025 -filter-month 11

  # Override Gmail query
  gmail-billing -query "from:amazon has:attachment"

  # Force reprocessing even if already processed
  gmail-billing -force-reprocess

  # Use a different config file
  gmail-billing -config ~/my-config.yaml
`)
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
	atts, err := gmailclient.DownloadAttachmentsToDir(srv, user, messageID, dir)
	if err != nil {
		logging.Errorf("Failed downloading attachments for %s: %v", messageID, err)
		return
	}

	if len(atts) == 0 {
		logging.Infof("No attachments downloaded for %s", messageID)
		return
	}

	from, subject, err := gmailclient.GetFromAndSubject(srv, user, messageID)
	if err != nil {
		logging.Errorf("Failed reading headers for %s: %v", messageID, err)
		// seguimos con fallbacks
	}

	company := gmailclient.DetectCompany(atts, from, subject)

	runTS := time.Now().Format("20060102T150405.000")
	runTS = strings.ReplaceAll(runTS, ".", "")

	if err := gmailclient.RenameDownloadedAttachments(dir, company, msgDate, runTS, atts); err != nil {
		logging.Errorf("Failed renaming attachments for %s: %v", messageID, err)
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

	// ---------------- FLAGS ----------------
	help := flag.Bool("help", false, "Show help message")
	helpShort := flag.Bool("h", false, "Show help message")

	configPath := flag.String("config", "config/config.yaml", "Path to YAML configuration file")
	cliQuery := flag.String("query", "", "Override Gmail search query (optional)")
	cliFilterYear := flag.Int("filter-year", 0, "Override filter year (optional)")
	cliFilterMonth := flag.Int("filter-month", 0, "Override filter month (1-12, optional)")
	cliDownloadOnly := flag.Bool("download-only", false, "Run in download-only mode (override config, optional)")
	cliForceReprocess := flag.Bool("force-reprocess", false, "Force reprocessing even if already processed")

	for _, arg := range os.Args {
		if arg == "--help" {
			printHelp()
			return
		}
	}

	flag.Parse()

	// Show help if requested
	if *help || *helpShort {
		printHelp()
		return
	}
	// ---------------------------------------

	logging.Infof("Starting Gmail organizer...")

	// ------------------------------------------------------------
	// 1) Load config
	// ------------------------------------------------------------
	cfg, err := config.Load(*configPath)
	if err != nil {
		logging.Fatalf("Failed to load config from %s: %v", *configPath, err)
	}

	logging.Infof(
		"Config loaded. user=%s query=%s max_results=%d base_label=%s invoices_path=%s filter_year=%d filter_month=%d download_only=%t",
		cfg.Gmail.User, cfg.Gmail.Query, cfg.Gmail.MaxResults,
		cfg.Gmail.BaseBillingLabel, cfg.Paths.BaseInvoicesPath,
		cfg.Gmail.FilterYear, cfg.Gmail.FilterMonth, cfg.Gmail.DownloadOnly,
	)

	if *cliQuery != "" {
		cfg.Gmail.Query = *cliQuery
	}
	if *cliFilterYear != 0 {
		cfg.Gmail.FilterYear = *cliFilterYear
	}
	if *cliFilterMonth != 0 {
		cfg.Gmail.FilterMonth = *cliFilterMonth
	}
	if *cliDownloadOnly {
		cfg.Gmail.DownloadOnly = true
	}
	if *cliForceReprocess {
		cfg.Gmail.ForceReprocess = true
	}

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
			if gmailclient.IsInvalidGrant(err) {
				logging.Infof("Token expirado o revocado (invalid_grant). Eliminando %s y reautenticando...", gmailclient.TokenFile)
				if rmErr := gmailclient.RemoveTokenFile(); rmErr != nil {
					logging.Fatalf("Failed to remove invalid token file: %v", rmErr)
				}
				srv, err = gmailclient.NewService(ctx, "credentials.json", gmail.GmailModifyScope)
				if err != nil {
					logging.Fatalf("Failed to create Gmail service after reauth: %v", err)
				}
				labelService, err = gmailclient.NewLabelService(srv, user)
			}
			if err != nil {
				logging.Fatalf("Failed to init LabelService: %v", err)
			}
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
	invalidGrantRetries := 0

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
			if gmailclient.IsInvalidGrant(err) {
				invalidGrantRetries++
				if invalidGrantRetries > 1 {
					logging.Fatalf("Token invalid_grant even after reauth. Aborting.")
				}
				logging.Infof("Token expirado o revocado (invalid_grant). Eliminando %s y reautenticando...", gmailclient.TokenFile)
				if rmErr := gmailclient.RemoveTokenFile(); rmErr != nil {
					logging.Fatalf("Failed to remove invalid token file: %v", rmErr)
				}
				srv, err = gmailclient.NewService(ctx, "credentials.json", gmail.GmailModifyScope)
				if err != nil {
					logging.Fatalf("Failed to create Gmail service after reauth: %v", err)
				}
				if !cfg.Gmail.DownloadOnly {
					labelService, err = gmailclient.NewLabelService(srv, user)
					if err != nil {
						logging.Fatalf("Failed to init LabelService after reauth: %v", err)
					}
					processedLabelID, err = labelService.EnsureLabel(processedLabelName)
					if err != nil {
						logging.Fatalf("Cannot ensure processed label after reauth: %v", err)
					}
					logging.Infof("Using Processed label: %s (ID=%s)", processedLabelName, processedLabelID)
				}
				pageNum--
				continue
			}
			logging.Fatalf("Failed listing messages: %v", err)
		}
		invalidGrantRetries = 0

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
