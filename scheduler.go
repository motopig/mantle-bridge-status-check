package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	crosschain "mantle-claim-crossing/cross_chain"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/robfig/cron/v3"
)

const (
	// L2OutputOracle contract address
	L2OutputOracleAddress = "0x31d543e7BE1dA6eFDc2206Ef7822879045B9f481"
	
	// OutputProposed event topic
	// event OutputProposed(bytes32 indexed outputRoot, uint256 indexed l2OutputIndex, uint256 indexed l2BlockNumber, uint256 l1Timestamp)
	OutputProposedTopic = "0xa7aaf2512769da4e444e3de247be2564225c2e7a8f74cfe528e46e17d24868e2"
)

// WithdrawalStatus tracks status for each withdrawal transaction
type WithdrawalStatus struct {
	sentWaitingMessage  bool // Track if we've sent the initial waiting message
	sent5MinuteReminder bool // Track if we've sent the 5-minute reminder
	finalized           bool // Track if this withdrawal has been finalized
}

// WithdrawalScheduler manages periodic checks for withdrawals
type WithdrawalScheduler struct {
	messenger            *crosschain.CrossChainMessenger
	l1Client             *ethclient.Client
	ctx                  context.Context
	cancel               context.CancelFunc
	telegramBot          *tgbotapi.BotAPI
	telegramChatID       int64
	telegramTopicID      int64                     // Topic ID for supergroups (0 for regular chats)
	withdrawalHashes     []string                  // List of withdrawal transaction hashes to monitor
	withdrawalStatus     map[string]*WithdrawalStatus // Status for each withdrawal
}

// NewWithdrawalScheduler creates a new scheduler
func NewWithdrawalScheduler() (*WithdrawalScheduler, error) {
	// Get RPC URLs from environment variables
	l1RpcUrl := os.Getenv("L1_RPC")
	l2RpcUrl := os.Getenv("L2_RPC")
	
	if l1RpcUrl == "" {
		return nil, fmt.Errorf("L1_RPC environment variable is not set")
	}
	if l2RpcUrl == "" {
		return nil, fmt.Errorf("L2_RPC environment variable is not set")
	}
	
	messenger, err := crosschain.CreateCrossChainMessenger(l1RpcUrl, l2RpcUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to create messenger: %w", err)
	}

	l1Client, err := ethclient.Dial(l1RpcUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to L1: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize Telegram bot (optional)
	var bot *tgbotapi.BotAPI
	var chatID int64
	var topicID int64
	telegramToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	telegramChatIDStr := os.Getenv("TELEGRAM_CHAT_ID")
	telegramTopicIDStr := os.Getenv("TELEGRAM_TOPIC_ID")
	
	if telegramToken != "" && telegramChatIDStr != "" {
		var err error
		bot, err = tgbotapi.NewBotAPI(telegramToken)
		if err != nil {
			log.Printf("⚠️  Warning: Failed to initialize Telegram bot: %v", err)
			log.Println("Continuing without Telegram notifications...")
		} else {
			fmt.Sscanf(telegramChatIDStr, "%d", &chatID)
			if telegramTopicIDStr != "" {
				fmt.Sscanf(telegramTopicIDStr, "%d", &topicID)
				log.Printf("✅ Telegram bot initialized: @%s (Topic ID: %d)", bot.Self.UserName, topicID)
			} else {
				log.Printf("✅ Telegram bot initialized: @%s", bot.Self.UserName)
			}
		}
	} else {
		log.Println("ℹ️  Telegram notifications disabled (TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID not set)")
	}

	// Parse withdrawal hashes from environment variable (comma-separated)
	var withdrawalHashes []string
	txHashesEnv := os.Getenv("WITHDRAWAL_TX_HASH")
	if txHashesEnv != "" {
		// Split by comma and trim whitespace
		for _, hash := range splitAndTrim(txHashesEnv, ",") {
			if hash != "" {
				withdrawalHashes = append(withdrawalHashes, hash)
			}
		}
	}

	// Initialize status map for each withdrawal
	withdrawalStatus := make(map[string]*WithdrawalStatus)
	for _, hash := range withdrawalHashes {
		withdrawalStatus[hash] = &WithdrawalStatus{}
	}

	return &WithdrawalScheduler{
		messenger:        messenger,
		l1Client:         l1Client,
		ctx:              ctx,
		cancel:           cancel,
		telegramBot:      bot,
		telegramChatID:   chatID,
		telegramTopicID:  topicID,
		withdrawalHashes: withdrawalHashes,
		withdrawalStatus: withdrawalStatus,
	}, nil
}

// splitAndTrim splits a string by delimiter and trims whitespace
func splitAndTrim(s, delimiter string) []string {
	parts := strings.Split(s, delimiter)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// sendTelegramMessage sends a notification via Telegram
func (s *WithdrawalScheduler) sendTelegramMessage(message string) {
	fmt.Println("Sending Telegram message")
	if s.telegramBot == nil || s.telegramChatID == 0 {
		return
	}
	fmt.Printf("Sending Telegram message: %s\n", message)
	msg := tgbotapi.NewMessage(s.telegramChatID, message)
	msg.ParseMode = "Markdown"
	
	// Set message thread ID if topic is specified (for supergroups)
	if s.telegramTopicID != 0 {
		msg.ReplyToMessageID = int(s.telegramTopicID)
	}
	
	if _, err := s.telegramBot.Send(msg); err != nil {
		log.Printf("⚠️  Failed to send Telegram message: %v", err)
	}
}

// GetLatestProposedL2Block gets the latest L2 block number from OutputProposed events
func (s *WithdrawalScheduler) GetLatestProposedL2Block() (uint64, error) {
	// Get the latest block number
	latestBlock, err := s.l1Client.BlockNumber(s.ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get latest block: %w", err)
	}

	// Query logs from the last 1000 blocks (about 3-4 hours)
	fromBlock := big.NewInt(int64(latestBlock - 1000))
	toBlock := big.NewInt(int64(latestBlock))

	// Create filter query
	query := ethereum.FilterQuery{
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Addresses: []common.Address{common.HexToAddress(L2OutputOracleAddress)},
		Topics:    [][]common.Hash{{common.HexToHash(OutputProposedTopic)}},
	}

	// Query logs
	logs, err := s.l1Client.FilterLogs(s.ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to filter logs: %w", err)
	}

	if len(logs) == 0 {
		return 0, fmt.Errorf("no OutputProposed events found in recent blocks")
	}

	// Get the latest event (last one in the array)
	latestLog := logs[len(logs)-1]

	// Parse the l2BlockNumber from topics
	// Event signature: OutputProposed(bytes32 indexed outputRoot, uint256 indexed l2OutputIndex, uint256 indexed l2BlockNumber, uint256 l1Timestamp)
	// topics[0] = event signature
	// topics[1] = outputRoot
	// topics[2] = l2OutputIndex
	// topics[3] = l2BlockNumber
	if len(latestLog.Topics) < 4 {
		return 0, fmt.Errorf("invalid log format: expected 4 topics, got %d", len(latestLog.Topics))
	}

	l2BlockNumber := new(big.Int).SetBytes(latestLog.Topics[3].Bytes()).Uint64()

	log.Printf("📊 Latest proposed L2 block: %d (L1 block: %d)", l2BlockNumber, latestLog.BlockNumber)
	return l2BlockNumber, nil
}

// CheckWithdrawal checks the withdrawal transaction and proves it if ready
func (s *WithdrawalScheduler) CheckWithdrawal(txHash string) error {
	if txHash == "" {
		return nil
	}

	log.Printf("🔍 Checking withdrawal: %s", txHash)

	// Get status for this withdrawal
	status := s.withdrawalStatus[txHash]
	if status == nil {
		status = &WithdrawalStatus{}
		s.withdrawalStatus[txHash] = status
	}

	// Get the L2 block number for this transaction
	message, err := s.messenger.GetMessages(s.ctx, txHash)
	if err != nil {
		return fmt.Errorf("failed to get message: %w", err)
	}

	log.Printf("  L2 Block: %d", message.BlockNumber)

	// Get latest proposed L2 block
	latestProposedBlock, err := s.GetLatestProposedL2Block()
	if err != nil {
		return fmt.Errorf("failed to get latest proposed block: %w", err)
	}

	log.Printf("  Latest Proposed: %d", latestProposedBlock)

	// Check if the withdrawal can be proven
	if latestProposedBlock >= message.BlockNumber {
		log.Printf("✅ Withdrawal is ready to prove!")
		
		log.Printf("  Current status: %d (%s)", message.Status, getStatusDescription(message.Status))

		// If already finalized, skip
		if message.Status >= 2 {
			log.Printf("  Already finalized, no action needed")
			
			// Mark as finalized if not already marked
			if !status.finalized {
				status.finalized = true
			}
			
			s.sendTelegramMessage(fmt.Sprintf(
				"✅ *Already Finalized*\n\n"+
				"Transaction: `%s`\n"+
				"Status: %s",
				txHash, getStatusDescription(message.Status)))
			return nil
		}

		// If already proven, check if it can be finalized
		if message.Status == 1 {
			log.Printf("  Already proven, checking if can be finalized...")
			
			// Check proven status to get the timestamp
			withdrawalHash := s.messenger.GetWithdrawalHash(message)
			isProven, provenTimestamp, err := s.messenger.CheckProvenStatus(s.ctx, withdrawalHash)
			if err != nil {
				return fmt.Errorf("failed to check proven status: %w", err)
			}
			
			if !isProven {
				log.Printf("  Warning: status is PROVEN but checkProvenStatus returned false")
				return nil
			}

			// Challenge period is 12 hours (43200 seconds)
			const challengePeriod = 12 * 60 * 60 // 12 hours in seconds
			currentTime := time.Now().Unix()
			finalizeTime := provenTimestamp.Int64() + challengePeriod
			
			if currentTime >= finalizeTime {
				log.Printf("✅ Challenge period has passed, ready to finalize!")
				
				// Reset flags for this withdrawal
				status.sentWaitingMessage = false
				status.sent5MinuteReminder = false
				
				// Send Telegram notification that withdrawal is ready to finalize
				s.sendTelegramMessage(fmt.Sprintf(
					"🎯 *Withdrawal Ready to Finalize*\n\n"+
					"Transaction: `%s`\n"+
					"Proven at: %s\n"+
					"Challenge period has passed!",
					txHash, time.Unix(provenTimestamp.Int64(), 0).Format(time.RFC3339)))
				
				// Attempt to finalize
				log.Printf("🚀 Attempting to finalize withdrawal...")
				s.sendTelegramMessage(fmt.Sprintf(
					"🚀 *Starting Finalize Operation*\n\n"+
					"Transaction: `%s`\n"+
					"Submitting finalization to L1...",
					txHash))
				
				err = s.messenger.FinalizeMessage(s.ctx, txHash, 0)
				if err != nil {
					log.Printf("❌ Failed to finalize: %v", err)
					s.sendTelegramMessage(fmt.Sprintf(
						"❌ *Finalize Failed*\n\n"+
						"Transaction: `%s`\n"+
						"Error: %v",
						txHash, err))
					return fmt.Errorf("failed to finalize: %w", err)
				}

				log.Printf("✅ Successfully finalized withdrawal!")
				s.sendTelegramMessage(fmt.Sprintf(
					"✅ *Finalize Successful!*\n\n"+
					"Transaction: `%s`\n"+
					"The withdrawal has been successfully finalized on L1!\n"+
					"Funds are now available.",
					txHash))

				// Mark this withdrawal as finalized
				status.finalized = true
				
				// Check if all withdrawals are finalized
				allFinalized := true
				for _, ws := range s.withdrawalStatus {
					if !ws.finalized {
						allFinalized = false
						break
					}
				}
				
				if allFinalized {
					// All withdrawals are finalized, stop the scheduler
					log.Printf("🛑 All withdrawals finalized — stopping scheduler and exiting cron")
					s.sendTelegramMessage("🎉 *All Withdrawals Completed!*\n\nAll configured withdrawals have been successfully finalized.")
					s.Stop()
				} else {
					log.Printf("✅ Withdrawal finalized, continuing to monitor remaining withdrawals...")
				}
				
				return nil
			} else {
				remainingTime := finalizeTime - currentTime
				finalizeTimeStr := time.Unix(finalizeTime, 0).Format(time.RFC3339)
				hours := remainingTime / 3600
				minutes := (remainingTime % 3600) / 60
				
				log.Printf("⏳ Challenge period not yet passed")
				log.Printf("   Can finalize at: %s (in %dh %dm)", finalizeTimeStr, hours, minutes)
				
				// Send Telegram message only:
				// 1. First time (initial waiting message)
				// 2. When there's 5 minutes remaining (reminder)
				const fiveMinutes = 5 * 60
				
				if !status.sentWaitingMessage {
					// Send initial waiting message
					s.sendTelegramMessage(fmt.Sprintf(
						"⏳ *Waiting for Challenge Period*\n\n"+
						"Transaction: `%s`\n"+
						"Status: PROVEN\n"+
						"Can finalize at: %s\n"+
						"Time remaining: %dh %dm",
						txHash, finalizeTimeStr, hours, minutes))
					status.sentWaitingMessage = true
				} else if remainingTime <= fiveMinutes && !status.sent5MinuteReminder {
					// Send 5-minute reminder
					s.sendTelegramMessage(fmt.Sprintf(
						"⏰ *Finalize Coming Soon*\n\n"+
						"Transaction: `%s`\n"+
						"Can finalize at: %s\n"+
						"Time remaining: %d minutes",
						txHash, finalizeTimeStr, minutes))
					status.sent5MinuteReminder = true
				}
			}
			return nil
		}

		// Status is READY_TO_PROVE, proceed with proving
		// Send Telegram notification that withdrawal is ready
		s.sendTelegramMessage(fmt.Sprintf(
			"🎯 *Withdrawal Ready to Prove*\n\n"+
			"Transaction: `%s`\n"+
			"L2 Block: %d\n"+
			"Latest Proposed: %d\n\n"+
			"The withdrawal is now ready to be proven!",
			txHash, message.BlockNumber, latestProposedBlock))
		
		// Attempt to prove
		log.Printf("🚀 Attempting to prove withdrawal...")
		s.sendTelegramMessage(fmt.Sprintf(
			"🚀 *Starting Prove Operation*\n\n"+
			"Transaction: `%s`\n"+
			"Submitting proof to L1...",
			txHash))
		
		err = s.messenger.ProveMessage(s.ctx, txHash, 0)
		if err != nil {
			log.Printf("❌ Failed to prove: %v", err)
			s.sendTelegramMessage(fmt.Sprintf(
				"❌ *Prove Failed*\n\n"+
				"Transaction: `%s`\n"+
				"Error: %v",
				txHash, err))
			return fmt.Errorf("failed to prove: %w", err)
		}

		log.Printf("✅ Successfully proved withdrawal!")
		
		// Calculate when it can be finalized (12 hours from now)
		const challengePeriod = 12 * 60 * 60
		finalizeTime := time.Now().Unix() + challengePeriod
		finalizeTimeStr := time.Unix(finalizeTime, 0).Format(time.RFC3339)
		
		s.sendTelegramMessage(fmt.Sprintf(
			"✅ *Prove Successful!*\n\n"+
			"Transaction: `%s`\n"+
			"L2 Block: %d\n\n"+
			"The withdrawal has been successfully proven on L1.\n"+
			"Can finalize at: %s (~12 hours)",
			txHash, message.BlockNumber, finalizeTimeStr))
	} else {
		remainingBlocks := message.BlockNumber - latestProposedBlock
		log.Printf("⏳ Still waiting: need %d more L2 blocks to be proposed", remainingBlocks)
		s.sendTelegramMessage(fmt.Sprintf(
			"⏳ *Prove Pending!*\n\n"+
			"Transaction: `%s`\n"+
			"Still waiting: need `%d` more L2 blocks to be proposed\n"+
			"Last Proposed Block: %d\n\n",
			txHash, remainingBlocks, latestProposedBlock))
	}

	return nil
}

// Start begins the periodic checking
func (s *WithdrawalScheduler) Start() {
	log.Println("🚀 Starting withdrawal scheduler (check interval: every 10 minutes)")
	
	// Create a new cron scheduler
	c := cron.New()
	
	// Add the check job to run every 10 minutes
	// Using cron expression: "*/10 * * * *" means every 10 minutes
	_, err := c.AddFunc("*/10 * * * *", func() {
		log.Printf("\n⏰ Running scheduled check at %s...", time.Now().Format(time.RFC3339))
		s.CheckAllWithdrawals()
	})
	
	if err != nil {
		log.Fatalf("Failed to add cron job: %v", err)
	}
	
	// Perform initial check
	log.Println("\n⏰ Performing initial check...")
	s.CheckAllWithdrawals()
	
	// Start the cron scheduler
	c.Start()
	log.Println("✅ Cron scheduler started")
	
	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for shutdown signal
	select {
	case <-sigChan:
		log.Println("\n🛑 Received shutdown signal, stopping scheduler...")
		c.Stop()
		s.cancel()
		return

	case <-s.ctx.Done():
		log.Println("🛑 Context cancelled, stopping scheduler...")
		c.Stop()
		return
	}
}

// CheckAllWithdrawals checks all withdrawal transactions
func (s *WithdrawalScheduler) CheckAllWithdrawals() {
	if len(s.withdrawalHashes) == 0 {
		log.Println("ℹ️  No withdrawal transactions to check (WITHDRAWAL_TX_HASH not set)")
		return
	}

	log.Printf("📋 Checking %d withdrawal(s)...", len(s.withdrawalHashes))
	
	for i, txHash := range s.withdrawalHashes {
		log.Printf("\n[%d/%d] Checking withdrawal: %s", i+1, len(s.withdrawalHashes), txHash)
		time.Sleep(30 * time.Second)
		if err := s.CheckWithdrawal(txHash); err != nil {
			log.Printf("❌ Check failed for %s: %v", txHash, err)
		}
	}
}

// Stop stops the scheduler
func (s *WithdrawalScheduler) Stop() {
	log.Println("🛑 Stopping scheduler...")
	s.cancel()
}

// getStatusDescription returns human-readable status description
func getStatusDescription(status int) string {
	switch status {
	case 0:
		return "READY_TO_PROVE"
	case 1:
		return "PROVEN"
	case 2:
		return "FINALIZED"
	default:
		return "UNKNOWN"
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	
	log.Println("=== Mantle Withdrawal Scheduler ===")
	log.Println()

	// Create scheduler
	scheduler, err := NewWithdrawalScheduler()
	if err != nil {
		log.Fatalf("Failed to create scheduler: %v", err)
	}

	// Check command line arguments
	if len(os.Args) < 2 {
		log.Println("Usage:")
		log.Println("  go run scheduler.go check             - Run a single check")
		log.Println("  go run scheduler.go start             - Start the scheduler")
		log.Println()
		log.Println("Environment Variables:")
		log.Println("  WITHDRAWAL_TX_HASH - Withdrawal transaction hash(es) to monitor (comma-separated for multiple)")
		log.Println()
		log.Println("Examples:")
		log.Println("  # Single withdrawal")
		log.Println("  export WITHDRAWAL_TX_HASH=0x2ddc5affc8b98cf6c9e5157347d726d0b11c79e9697a3d27ec55aa9693f9baf2")
		log.Println("  # Multiple withdrawals")
		log.Println("  export WITHDRAWAL_TX_HASH=0x2ddc5affc8b98cf6c9e5157347d726d0b11c79e9697a3d27ec55aa9693f9baf2,0xabc123...")
		log.Println("  go run scheduler.go check")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "check":
		log.Println("🔍 Running single check...")
		scheduler.CheckAllWithdrawals()

	case "start":
		log.Println("🚀 Starting scheduler in continuous mode...")
		scheduler.Start()

	default:
		log.Fatalf("Unknown command: %s (use 'check' or 'start')", command)
	}
}
