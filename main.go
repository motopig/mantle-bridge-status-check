package main

import (
	"context"
	"fmt"
	"log"
	crosschain "mantle-claim-crossing/cross_chain"
	"os"
	"strconv"
	"strings"
)



func main() {
	args := os.Args[1:]

	if len(args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := strings.ToLower(args[0])
	txHash := args[1]
	messageIndex := 0

	if len(args) > 2 {
		if idx, err := strconv.Atoi(args[2]); err == nil {
			messageIndex = idx
		}
	}
	
	// 0 ready to prove
    // 1 proven
	// 2 relayed/finalized
    // 3 finalize message

	// Create messenger with real RPC endpoints and KMS support
	messenger, err := crosschain.CreateCrossChainMessenger(
		os.Getenv("L1_RPC"),
		os.Getenv("L2_RPC"),
	)
	if err != nil {
		log.Fatalf("Failed to create messenger: %v", err)
	}

	ctx := context.Background()

	switch command {
	case "check", "status":
		err = messenger.CheckMessageStatus(ctx, txHash, messageIndex)
	case "prove":
		err = messenger.ProveMessage(ctx, txHash, messageIndex)
	case "finalize", "claim":
		err = messenger.FinalizeMessage(ctx, txHash, messageIndex)
	case "can-finalize", "ready":
		// err = messenger.CheckFinalizeReadiness(ctx, txHash, messageIndex)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		log.Fatalf("\n❌ Operation failed: %v", err)
	}

	fmt.Println("\n✅ Operation completed successfully")
}

func printUsage() {
	fmt.Println("Mantle Cross-Chain Message Status Checker with AWS KMS Support")
	fmt.Println("Usage: go run main.go <command> <tx_hash> [message_index]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  check/status     - Check message status")
	fmt.Println("  prove            - Prove message")
	fmt.Println("  finalize/claim   - Finalize message")
	fmt.Println("  can-finalize/ready - Check if ready to finalize")
	fmt.Println("  full             - Full claim process")
	fmt.Println("")
	fmt.Println("Environment Variables:")
	fmt.Println("  KMS_KEY_ID       - AWS KMS Key ID for signing (recommended)")
	fmt.Println("  PRIV_KEY         - Private key for signing (alternative)")
	fmt.Println("  AWS_REGION       - AWS region (default: ap-northeast-1)")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  go run main.go check 0xe0c400563d9a70f84966622f13a5560bfacfe9621ea554ee7939fd06d2e10417")
	fmt.Println("  go run main.go can-finalize 0xe0c400563d9a70f84966622f13a5560bfacfe9621ea554ee7939fd06d2e10417")
	fmt.Println("")
	fmt.Println("Setup:")
	fmt.Println("  1. Copy .env.example to .env")
	fmt.Println("  2. Set either KMS_KEY_ID or PRIV_KEY in .env")
	fmt.Println("  3. Ensure AWS credentials are configured (for KMS)")
}
