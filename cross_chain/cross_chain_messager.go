package crosschain

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	cross_abi "mantle-claim-crossing/abi"
	"mantle-claim-crossing/helper"
	"math/big"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rlp"
	kmssigner "github.com/welthee/go-ethereum-aws-kms-tx-signer/v2"
	"golang.org/x/crypto/sha3"
)

// CreateCrossChainMessenger creates a new CrossChainMessenger with KMS or private key support
func CreateCrossChainMessenger(l1RpcUrl, l2RpcUrl string) (*CrossChainMessenger, error) {
	messenger := &CrossChainMessenger{
		L1RpcUrl: l1RpcUrl,
		L2RpcUrl: l2RpcUrl,
	}
	contracts := CrossChainContracts{
		L1: L1Contracts{
			StateCommitmentChain:   getEnvOrDefault("L1_STATE_COMMITMENT_CHAIN", "0x0000000000000000000000000000000000000000"),
			CanonicalTransactionChain: getEnvOrDefault("L1_CANONICAL_TRANSACTION_CHAIN", "0x0000000000000000000000000000000000000000"),
			BondManager:            getEnvOrDefault("L1_BOND_MANAGER", "0x0000000000000000000000000000000000000000"),
			AddressManager:         getEnvOrDefault("L1_ADDRESS_MANAGER", "0x6968f3F16C3e64003F02E121cf0D5CCBf5625a42"),
			L1CrossDomainMessenger: getEnvOrDefault("L1_CROSS_DOMAIN_MESSENGER", "0x676A795fe6E43C17c668de16730c3F690FEB7120"),
			L1StandardBridge:       getEnvOrDefault("L1_STANDARD_BRIDGE", "0x95fC37A27a2f68e3A647CDc081F0A89bb47c3012"),
			OptimismPortal:         getEnvOrDefault("L1_OPTIMISM_PORTAL", "0xc54cb22944F2bE476E02dECfCD7e3E7d3e15A8Fb"),
			L2OutputOracle:         getEnvOrDefault("L2_OUTPUT_ORACLE", "0x31d543e7BE1dA6eFDc2206Ef7822879045B9f481"),
		},
		Bridges: BridgeContracts{
			L1Bridge: getEnvOrDefault("L1_BRIDGE", "0x95fC37A27a2f68e3A647CDc081F0A89bb47c3012"),
			L2Bridge: getEnvOrDefault("L2_BRIDGE", "0x4200000000000000000000000000000000000010"),
			L2CrossDomainMessenger:  getEnvOrDefault("L2_CROSS_DOMAIN_MESSENGER", "0x4200000000000000000000000000000000000007"),
			L2ToL1MessagePasser: getEnvOrDefault("L2_TO_L1_MESSAGE_PASSER", "0x4200000000000000000000000000000000000016"),
		},
	}
	messenger.Contracts = contracts
	l1Client, err := ethclient.Dial(messenger.L1RpcUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to L1 RPC: %w", err)
	}
	messenger.ClientL1 = l1Client
	l2Client, err := ethclient.Dial(messenger.L2RpcUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to L2 RPC: %w", err)
	}
	messenger.ClientL2 = l2Client

	// Check for KMS key ID first
	kmsKeyID := os.Getenv("KMS_KEY_ID")
	privateKey := os.Getenv("PRIV_KEY")

	if kmsKeyID != "" {
		fmt.Println("🔐 Using AWS KMS for signing")
		
		// Load AWS config
		cfg, err := config.LoadDefaultConfig(context.TODO())
		if err != nil {
			return nil, fmt.Errorf("failed to load AWS config: %w", err)
		}

		// Create KMS client
		messenger.KMSClient = kms.NewFromConfig(cfg)
		messenger.KMSKeyID = kmsKeyID

		// Get wallet address from KMS using the library
		transactor, err := kmssigner.NewAwsKmsTransactorWithChainID(messenger.KMSClient, kmsKeyID, big.NewInt(1))
		if err != nil {
			return nil, fmt.Errorf("failed to create KMS transactor: %w", err)
		}
		
		messenger.WalletAddress = transactor.From.Hex()
		fmt.Printf("💼 Wallet address: %s\n", messenger.WalletAddress)
	} else if privateKey != "" {
		fmt.Println("🔑 Using private key for signing")
		messenger.PrivateKey = privateKey
		
		// Get wallet address from private key
		address, err := messenger.getWalletAddressFromPrivateKey()
		if err != nil {
			return nil, fmt.Errorf("failed to get wallet address from private key: %w", err)
		}
		messenger.WalletAddress = address
		fmt.Printf("💼 Wallet address: %s\n", address)
	} else {
		return nil, fmt.Errorf("either KMS_KEY_ID or PRIV_KEY environment variable must be set")
	}

	return messenger, nil
}

// CheckMessageStatus checks the status of a cross-chain message
func (m *CrossChainMessenger) CheckMessageStatus(ctx context.Context, txHash string, messageIndex int) error {
	fmt.Println("\n=== CHECK MESSAGE STATUS ===")
	fmt.Printf("🔍 Checking transaction: %s\n", txHash)
	fmt.Printf("📍 Message index: %d\n", messageIndex)

	message, err := m.getMessages(ctx, txHash)
	if err != nil {
		return fmt.Errorf("failed to get messages: %w", err)
	}

	fmt.Printf("\n📋 Message Details:\n")
	fmt.Printf("  Transaction Hash: %s\n", message.TxHash)
	fmt.Printf("  Block Number: %d\n", message.BlockNumber)
	fmt.Printf("  Log Index: %d\n", message.LogIndex)
	fmt.Printf("  Direction: %s\n", message.Direction)
	

	fmt.Printf("  Status: %d (%s)\n", message.Status, getStatusDescription(message.Status))

	return nil
}

// GetMessages retrieves cross-chain messages from a transaction (exported for external use)
func (m *CrossChainMessenger) GetMessages(ctx context.Context, txHash string) (Message, error) {
	return m.getMessages(ctx, txHash)
}

// getMessages retrieves cross-chain messages from a transaction
func (m *CrossChainMessenger) getMessages(ctx context.Context, txHash string) (Message, error) {
	fmt.Printf("🔍 Getting transaction receipt for: %s\n", txHash)

	// Get transaction receipt from L2
	receipt, err := m.getTransactionReceipt(ctx, txHash, "L2")
	
	if err != nil {
		return Message{}, fmt.Errorf("failed to get transaction receipt: %w", err)
	}

	// Parse logs to find cross-chain messages using enhanced parsing
	message, err := m.parseSentMessageLogsEnhanced(receipt)
	if err != nil {
		return message, fmt.Errorf("failed to parse logs: %w", err)
	}
	
	messagePassed, err := m.parseMessagePassedLogsEnhanced(receipt)
	message.MessagePassedEvent = messagePassed
	if err != nil {
		return message, fmt.Errorf("failed to parse parseMessagePassedLogsEnhanced: %w", err)
	}
	message.MsgNonce = messagePassed.Nonce
	message.WithdrawalHash = hex.EncodeToString(messagePassed.WithdrawalHash[:])
	message.SentMessageExtension1Event, err = m.parseSentMessageExtension1LogsEnhanced(receipt)

	if message.SentMessageExtension1Event != nil {
		if message.SentMessageExtension1Event.MntValue == nil {	
			message.MntValue = big.NewInt(0)
		}
		if message.SentMessageExtension1Event.EthValue == nil {
			message.EthValue = big.NewInt(0)
		}
		if message.SentMessageExtension1Event.MntValue != nil {
			message.MntValue = message.SentMessageExtension1Event.MntValue
		}
		if message.SentMessageExtension1Event.EthValue != nil {
			message.EthValue = message.SentMessageExtension1Event.EthValue
		}
	} else {
		// If no SentMessageExtension1 event, default to 0
		message.MntValue = big.NewInt(0)
		message.EthValue = big.NewInt(0)
	}

	
	status, err := m.getMessageStatus(ctx, &message)
	if err != nil {
		fmt.Printf("⚠️  Warning: Failed to get status for message : %v\n", err)
		
	}
	message.Status = status

	return message, nil
}

// getTransactionReceipt fetches transaction receipt from L2
func (m *CrossChainMessenger) getTransactionReceipt(ctx context.Context, txHash string, network string) (*types.Receipt, error) {
	var receipt *types.Receipt
	var err error
	if network == "L2" {
		receipt, err = m.ClientL2.TransactionReceipt(ctx, common.HexToHash(txHash))
	} else {
		receipt, err = m.ClientL1.TransactionReceipt(ctx, common.HexToHash(txHash))
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction receipt: %w", err)
	}
	
	return receipt, nil
}


// getMessageStatus determines the status of a cross-chain message
func (m *CrossChainMessenger) getMessageStatus(ctx context.Context, message *Message) (int, error) {
	fmt.Printf("🔍 Getting message status for tx: %s, log: %d\n", message.TxHash, message.LogIndex)
	
	fmt.Printf("\n🔍 Trying withdrawal hash method %d: %s\n", 1, message.WithdrawalHash)
	
	// Check if message is finalized
	isFinalized, err := m.checkFinalizationStatus(ctx, message.WithdrawalHash)
	if err != nil {
		fmt.Printf("❌ Failed to check finalization status: %v\n", err)
	} else {
		fmt.Printf("🏁 Finalization status: %t\n", isFinalized)
		if isFinalized {
			fmt.Printf("✅ Found correct withdrawal hash (method %d): %s\n", 1, message.WithdrawalHash)
			return 2, nil // RELAYED/FINALIZED
		}
	}

	// Check if message is proven
	isProven, timeStamp, err := m.checkProvenStatus(ctx, message.WithdrawalHash)
	
	if err != nil {
		fmt.Printf("❌ Failed to check proven status: %v\n", err)
	} else {
		fmt.Printf("✅ Proven status: %t\n", isProven)
		// proven time + 12 hours can finalize
		currentTimeStamp := *big.NewInt(getCurrentTimestamp())
		provenTimePlus12Hours := new(big.Int).Add(timeStamp, big.NewInt(43200))
		if currentTimeStamp.Cmp(provenTimePlus12Hours) >= 0 && timeStamp.Cmp(big.NewInt(0)) > 0 {
			fmt.Println("✅ Message can be finalized now.")
		} else if timeStamp.Cmp(big.NewInt(0)) == 0 {
			fmt.Println("⏳ Message is not yet proven.")
		} else {
			fmt.Println("⏳ Message cannot be finalized yet. Please wait for the challenge period to pass.")
		}
		if isProven {
			return 1, nil // PROVEN
		}
	}

	return 0, nil // READY_TO_PROVE
}

// checkFinalizationStatus checks if a message is finalized on L1
func (m *CrossChainMessenger) checkFinalizationStatus(ctx context.Context, withdrawalHash string) (bool, error) {

	op, err := cross_abi.NewOptimismPortal(common.HexToAddress(m.Contracts.L1.OptimismPortal), m.ClientL1)
	if err != nil {
		return false, err
	}
	result, err := op.FinalizedWithdrawals(nil, common.HexToHash(withdrawalHash))
	if err != nil {
		return false, err
	}
	fmt.Printf("📤 checkFinalizationStatus result: %t\n", result)	
	return result, nil
}

// checkProvenStatus checks if a message is proven on L1
func (m *CrossChainMessenger) checkProvenStatus(ctx context.Context, withdrawalHash string) (bool, *big.Int, error) {
	op, _ := cross_abi.NewOptimismPortal(common.HexToAddress(m.Contracts.L1.OptimismPortal), m.ClientL1)
	result, err := op.ProvenWithdrawals(nil, common.HexToHash(withdrawalHash))
	if err != nil {
		return false, nil, err
	}
	
	fmt.Printf("📤 checkProvenStatus result: %s\n", result)
	// If result is all zeros, withdrawal is not proven
	return common.Bytes2Hex(result.OutputRoot[:]) != "0000000000000000000000000000000000000000000000000000000000000000", result.Timestamp, nil
}

// CheckProvenStatus is the exported version of checkProvenStatus
func (m *CrossChainMessenger) CheckProvenStatus(ctx context.Context, withdrawalHash string) (bool, *big.Int, error) {
	return m.checkProvenStatus(ctx, withdrawalHash)
}

// GetWithdrawalHash returns the withdrawal hash from a message
func (m *CrossChainMessenger) GetWithdrawalHash(message Message) string {
	return message.WithdrawalHash
}


// ProveMessage proves a cross-chain message
func (m *CrossChainMessenger) ProveMessage(ctx context.Context, txHash string, messageIndex int) error {
	fmt.Println("\n=== PROVE MESSAGE ===")
	fmt.Printf("Transaction hash (on L2): %s\n", txHash)
	fmt.Printf("Message index: %d\n", messageIndex)

	message, err := m.getMessages(ctx, txHash)
	if err != nil {
		return fmt.Errorf("failed to get messages: %w", err)
	}

	fmt.Printf("Message direction: %s\n", message.Direction)
	fmt.Printf("Message status: %d\n", message.Status)

	// Check if already proven
	if message.Status >= 2 { // TODO 1
		fmt.Println("✅ Message already proven or finalized")
		return nil
	}

	fmt.Println("🔄 Starting prove message...")

	// Get L2 output index
	l2OutputOracleAddress := m.Contracts.L1.L2OutputOracle
	outputIndex, err := m.getL2OutputIndex(ctx, l2OutputOracleAddress, message.BlockNumber)
	if err != nil {
		return fmt.Errorf("failed to get L2 output index: %w", err)
	}
	fmt.Printf("📊 L2 Output Index: %d\n", outputIndex)

	// Get L2 output data (output root proof)
	outputData, err := m.getL2OutputData(ctx, l2OutputOracleAddress, outputIndex)
	if err != nil {
		return fmt.Errorf("failed to get L2 output data: %w", err)
	}
	fmt.Printf("📊 Output Root: %s\n", common.Bytes2Hex(outputData.OutputRoot[:]))
	fmt.Printf("📊 L2 Block Number: %d\n", outputData.L2BlockNumber)

	// Parse withdrawal transaction parameters
	eventData := message.MessagePassedEvent
	if eventData == nil {
		return fmt.Errorf("event data is nil")
	}

	// Generate withdrawal proof
	// CRITICAL: The withdrawal must have been included in or before the L2 Output block
	// We generate the proof using the L2 Output block's state, not the transaction block
	fmt.Println("\n🔍 Generating withdrawal proof...")
	fmt.Printf("📍 Transaction block: %d, L2 Output block: %d\n", 
		message.BlockNumber, outputData.L2BlockNumber.Uint64())
	
	if message.BlockNumber > outputData.L2BlockNumber.Uint64() {
		return fmt.Errorf("transaction block %d is after L2 output block %d, need to wait for a newer output",
			message.BlockNumber, outputData.L2BlockNumber.Uint64())
	}
	
	withdrawalProof, err := m.generateWithdrawalProofForBlock(ctx, message, outputData.L2BlockNumber.Uint64())
	if err != nil {
		return fmt.Errorf("failed to generate withdrawal proof: %w", err)
	}

	// Build output root proof
	outputRootProof := cross_abi.TypesOutputRootProof{
		Version:                  [32]byte{}, // Version is typically 0
		StateRoot:                withdrawalProof.StateRoot,
		MessagePasserStorageRoot: withdrawalProof.MessagePasserStorageRoot,
		LatestBlockhash:          withdrawalProof.LatestBlockhash,
	}
	
	fmt.Printf("\n📊 Output Root Proof:\n")
	fmt.Printf("  Version: %x\n", outputRootProof.Version)
	fmt.Printf("  State Root: %x\n", outputRootProof.StateRoot)
	fmt.Printf("  Message Passer Storage Root: %x\n", outputRootProof.MessagePasserStorageRoot)
	fmt.Printf("  Latest Block Hash: %x\n", outputRootProof.LatestBlockhash)
	
	// Calculate and verify the output root
	// OutputRoot = keccak256(abi.encode(version, stateRoot, messagePasserStorageRoot, latestBlockhash))
	calculatedOutputRoot := m.calculateOutputRoot(outputRootProof)
	fmt.Printf("\n🔍 Calculated Output Root: %s\n", common.Bytes2Hex(calculatedOutputRoot[:]))
	fmt.Printf("🔍 Expected Output Root:   %s\n", common.Bytes2Hex(outputData.OutputRoot[:]))
	
	if calculatedOutputRoot != outputData.OutputRoot {
		return fmt.Errorf("output root mismatch: calculated %s, expected %s", 
			common.Bytes2Hex(calculatedOutputRoot[:]), 
			common.Bytes2Hex(outputData.OutputRoot[:]))
	}
	fmt.Println("✅ Output root verification passed!")

	// Build withdrawal transaction
	withdrawalTx := cross_abi.TypesWithdrawalTransaction{
		Nonce:    message.MsgNonce,
		Sender:   eventData.Sender,
		Target:   eventData.Target,
		MntValue: message.MntValue,
		EthValue: message.EthValue,
		GasLimit: eventData.GasLimit,
		Data:     eventData.Data,
	}

	fmt.Printf("\n📋 Withdrawal Transaction:\n")
	fmt.Printf("  Nonce: %s\n", withdrawalTx.Nonce.String())
	fmt.Printf("  Sender: %s\n", withdrawalTx.Sender.Hex())
	fmt.Printf("  Target: %s\n", withdrawalTx.Target.Hex())
	fmt.Printf("  MNT Value: %s\n", withdrawalTx.MntValue.String())
	fmt.Printf("  ETH Value: %s\n", withdrawalTx.EthValue.String())
	fmt.Printf("  Gas Limit: %s\n", withdrawalTx.GasLimit.String())
	fmt.Printf("  Data Length: %d bytes\n", len(withdrawalTx.Data))
	fmt.Printf("  Data: %x\n", withdrawalTx.Data)
	fmt.Println("outputIndex ", outputIndex)
	// Call proveWithdrawalTransaction
	fmt.Println("\n📤 Calling proveWithdrawalTransaction...")
	err = m.callProveWithdrawalTransaction(ctx, withdrawalTx, outputIndex, outputRootProof, withdrawalProof.WithdrawalProof)
	if err != nil {
		return fmt.Errorf("failed to prove withdrawal transaction: %w", err)
	}

	fmt.Println("✅ Message proved successfully!")
	return nil
}

// FinalizeMessage finalizes a cross-chain message
func (m *CrossChainMessenger) FinalizeMessage(ctx context.Context, txHash string, messageIndex int) error {
	fmt.Println("\n=== FINALIZE MESSAGE ===")
	fmt.Printf("Transaction hash (on L2): %s\n", txHash)
	fmt.Printf("Message index: %d\n", messageIndex)

	message, err := m.getMessages(ctx, txHash)
	if err != nil {
		return fmt.Errorf("failed to get messages: %w", err)
	}

	fmt.Printf("Message direction: %s\n", message.Direction)
	fmt.Printf("Message status: %d\n", message.Status)

	// Check if already finalized
	if message.Status >= 2 {
		fmt.Println("✅ Message already finalized")
		return nil
	}

	// Check if proven
	if message.Status < 1 {
		fmt.Println("❌ Message not proven yet. Run prove first.")
		return fmt.Errorf("message not proven")
	}

	fmt.Println("🔄 Starting finalize message...")
	
	// Parse event data to get withdrawal parameters
	eventData := message.MessagePassedEvent
	if eventData == nil {
		return fmt.Errorf("event data is nil")
	}

	// Construct withdrawal transaction using the generated struct from optimism_portal.go
	withdrawalTx := cross_abi.TypesWithdrawalTransaction{
		Nonce:    message.MsgNonce,
		Sender:   eventData.Sender,
		Target:   eventData.Target,
		MntValue: message.MntValue,
		EthValue: message.EthValue,
		GasLimit: eventData.GasLimit,
		Data:     eventData.Data,
	}

	fmt.Printf("\n📋 Withdrawal Transaction Parameters:\n")
	fmt.Printf("  Nonce: %s\n", withdrawalTx.Nonce.String())
	fmt.Printf("  Sender: %s\n", withdrawalTx.Sender.Hex())
	fmt.Printf("  Target: %s\n", withdrawalTx.Target.Hex())
	fmt.Printf("  MNT Value: %s\n", withdrawalTx.MntValue.String())
	fmt.Printf("  ETH Value: %s\n", withdrawalTx.EthValue.String())
	fmt.Printf("  Gas Limit: %s\n", withdrawalTx.GasLimit.String())
	fmt.Printf("  Data: %s\n", string(withdrawalTx.Data))

	// Create OptimismPortal contract instance
	optimismPortalAddr := common.HexToAddress(m.Contracts.L1.OptimismPortal)
	optimismPortal, err := cross_abi.NewOptimismPortal(optimismPortalAddr, m.ClientL1)
	if err != nil {
		return fmt.Errorf("failed to create OptimismPortal contract: %w", err)
	}

	fmt.Printf("\n📝 OptimismPortal address: %s\n", optimismPortalAddr.Hex())
	fmt.Printf("📝 Withdrawal hash: %s\n", message.WithdrawalHash)

	// Get transaction options
	txOpts, err := m.getTransactOpts(ctx)
	if err != nil {
		return fmt.Errorf("failed to get transaction options: %w", err)
	}

	// Send transaction using KMS or private key
	fmt.Println("\n🚀 Sending finalize transaction...")
	
	// Call finalizeWithdrawalTransaction
	tx, err := optimismPortal.FinalizeWithdrawalTransaction(txOpts, withdrawalTx)
	if err != nil {
		return fmt.Errorf("failed to finalize withdrawal transaction: %w", err)
	}

	fmt.Printf("✅ Finalize transaction submitted: %s\n", tx.Hash().Hex())
	
	// Print raw transaction data for manual broadcasting
	// txData, err := tx.MarshalBinary()
	// if err != nil {
	// 	fmt.Printf("⚠️  Failed to marshal transaction: %v\n", err)
	// } else {
	// 	fmt.Printf("\n📦 Raw Transaction Data (for manual broadcast):\n")
	// 	fmt.Printf("0x%x\n", txData)
	// 	fmt.Printf("\n💡 You can broadcast this with: cast publish 0x%x --rpc-url $L1_RPC\n", txData)
	// }
	
	fmt.Println("\n⏳ Waiting for transaction to be mined...")

	// Wait for transaction to be mined
	receipt, err := bind.WaitMined(ctx, m.ClientL1, tx)
	if err != nil {
		return fmt.Errorf("failed to wait for transaction: %w", err)
	}
	
	if receipt.Status == 0 {
		return fmt.Errorf("transaction failed (status: 0)")
	}
	
	fmt.Printf("✅ Transaction mined in block %d (status: %d)\n", receipt.BlockNumber.Uint64(), receipt.Status)
	fmt.Printf("   Gas used: %d\n", receipt.GasUsed)
	fmt.Printf("🔗 Check transaction: https://etherscan.io/tx/%s\n", tx.Hash().Hex())
	
	return nil
}


// getWalletAddressFromPrivateKey derives wallet address from private key
func (m *CrossChainMessenger) getWalletAddressFromPrivateKey() (string, error) {
	if m.PrivateKey == "" {
		return "", fmt.Errorf("private key not set")
	}

	// Remove 0x prefix if present
	privateKeyHex := strings.TrimPrefix(m.PrivateKey, "0x")
	
	// Parse private key
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode private key: %w", err)
	}

	// Create ECDSA private key
	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to create ECDSA private key: %w", err)
	}

	// Get public key and compute address
	publicKey := &privateKey.PublicKey
	address := publicKeyToAddress(publicKey)
	return address, nil
}


// publicKeyToAddress converts ECDSA public key to Ethereum address
func publicKeyToAddress(publicKey *ecdsa.PublicKey) string {
	// Serialize public key to uncompressed format
	publicKeyBytes := append(publicKey.X.Bytes(), publicKey.Y.Bytes()...)
	
	// Pad to 64 bytes if needed
	if len(publicKeyBytes) < 64 {
		padding := make([]byte, 64-len(publicKeyBytes))
		publicKeyBytes = append(padding, publicKeyBytes...)
	}
	
	// Compute Keccak256 hash
	hash := sha3.NewLegacyKeccak256()
	hash.Write(publicKeyBytes)
	hashBytes := hash.Sum(nil)
	
	// Take last 20 bytes as address
	address := "0x" + hex.EncodeToString(hashBytes[12:])
	return address
}

// SignWithKMS is deprecated - the library handles signing internally
// Kept for backward compatibility but no longer used
func (m *CrossChainMessenger) SignWithKMS(hash []byte) (*EthereumSignature, error) {
	return nil, fmt.Errorf("SignWithKMS is deprecated - use kmssigner library directly")
}

// getL2OutputIndex gets the L2 output index for a given block number
func (m *CrossChainMessenger) getL2OutputIndex(ctx context.Context, l2OutputOracleAddress string, blockNumber uint64) (uint64, error) {
	// getL2OutputIndexAfter(uint256 _l2BlockNumber) function selector: 0x7f006420
	functionSelector := "0x7f006420"
	
	// Convert block number to hex and pad to 32 bytes
	blockNumberHex := fmt.Sprintf("%064x", blockNumber)
	callData := functionSelector + blockNumberHex
	
	fmt.Printf("🔍 Getting L2 output index for block %d\n", blockNumber)
	fmt.Printf("📝 Call data: %s\n", callData)
	l2Oracle, err := cross_abi.NewL2OutputOracle(common.HexToAddress(l2OutputOracleAddress), m.ClientL1)
	if err != nil {
		return 0, fmt.Errorf("failed to create L2OutputOracle instance: %w", err)
	}
	result, err := 	l2Oracle.GetL2OutputIndexAfter(nil, big.NewInt(int64(blockNumber)))
	if err != nil {
		return 0, fmt.Errorf("failed to call getL2OutputIndexAfter: %w", err)
	}

	return result.Uint64(), nil
}


// getL2OutputData gets L2 output data for a given index
func (m *CrossChainMessenger) getL2OutputData(ctx context.Context, l2OutputOracleAddress string, outputIndex uint64) (cross_abi.TypesOutputProposal, error) {
	var result cross_abi.TypesOutputProposal
	l2Oracle, err := cross_abi.NewL2OutputOracle(common.HexToAddress(l2OutputOracleAddress), m.ClientL1)
	if err != nil {
		return result, fmt.Errorf("failed to create L2OutputOracle instance: %w", err)
	}

	result, err = l2Oracle.GetL2Output(nil, big.NewInt(int64(outputIndex)))
	
	return result, err
}



// checkCanFinalize checks if a proven withdrawal is ready to be finalized
func (m *CrossChainMessenger) checkCanFinalize(ctx context.Context, withdrawalHash string, message *Message) (bool, error) {
	fmt.Printf("🔍 Checking if withdrawal can be finalized...\n")
	fmt.Printf("📋 Block number: %d (0x%x)\n", message.BlockNumber, message.BlockNumber)
	
	// For Mantle, after a withdrawal is proven, there's typically a 12-hour challenge period
	// Let's try to get actual timing data, but fall back to heuristic if needed
	
	// L2OutputOracle contract address for Mantle
	l2OutputOracleAddress := "0x31d543e7BE1dA6eFDc2206Ef7822879045B9f481"
	fmt.Printf("📞 L2OutputOracle: %s\n", l2OutputOracleAddress)
	
	// Try to get L2 output index for this block number with timeout protection
	fmt.Printf("� Attempting to get L2 output index...\n")
	outputIndex, err := m.getL2OutputIndex(ctx, l2OutputOracleAddress, message.BlockNumber)
	if err != nil {
		fmt.Printf("⚠️  Failed to get L2 output index: %v\n", err)
		fmt.Printf("💡 Using heuristic: For proven withdrawals, assuming 12+ hours have passed\n")
		fmt.Printf("🚀 READY TO FINALIZE! (heuristic - proven withdrawals are typically ready)\n")
		return true, nil
	}
	
	fmt.Printf("✅ L2 Output Index: %d\n", outputIndex)
	
	// Try to get the output data with timestamp
	fmt.Printf("🔍 Attempting to get L2 output data...\n")
	outputData, err := m.getL2OutputData(ctx, l2OutputOracleAddress, outputIndex)
	if err != nil {
		fmt.Printf("⚠️  Failed to get L2 output data: %v\n", err)
		fmt.Printf("💡 Using heuristic: For proven withdrawals, assuming 12+ hours have passed\n")
		fmt.Printf("� READY TO FINALIZE! (heuristic - proven withdrawals are typically ready)\n")
		return true, nil
	}
	
	// Calculate if 12 hours have passed
	challengePeriod := int64(12 * 60 * 60) // 12 hours in seconds
	currentTime := getCurrentTimestamp()
	timeElapsed := currentTime - outputData.Timestamp.Int64()
	
	// Output timing information
	
	fmt.Printf("⏰ Current timestamp: %d\n", currentTime)
	fmt.Printf("⏰ Output timestamp: %d\n", outputData.Timestamp)
	fmt.Printf("⏰ Time elapsed: %d seconds (%.1f hours)\n", timeElapsed, float64(timeElapsed)/3600.0)
	fmt.Printf("⏰ Challenge period: %d seconds (12 hours)\n", challengePeriod)
	
	canFinalize := timeElapsed >= challengePeriod
	
	if canFinalize {
		fmt.Printf("🚀 READY TO FINALIZE! Challenge period has passed (%.1f hours elapsed)\n", float64(timeElapsed)/3600.0)
	} else {
		remainingTime := challengePeriod - timeElapsed
		fmt.Printf("⏳ STILL IN CHALLENGE PERIOD: Need to wait %.1f more hours\n", float64(remainingTime)/3600.0)
	}
	
	return canFinalize, nil
}

// generateWithdrawalProof generates the withdrawal proof for a message using eth_getProof
func (m *CrossChainMessenger) generateWithdrawalProof(ctx context.Context, message Message) (*WithdrawalProof, error) {
	return m.generateWithdrawalProofForBlock(ctx, message, message.BlockNumber)
}

// generateWithdrawalProofForBlock generates the withdrawal proof for a specific block number
func (m *CrossChainMessenger) generateWithdrawalProofForBlock(ctx context.Context, message Message, blockNumber uint64) (*WithdrawalProof, error) {
	fmt.Println("🔍 Generating withdrawal proof using eth_getProof...")
	
	// L2ToL1MessagePasser contract address
	messagePasserAddr := common.HexToAddress(m.Contracts.Bridges.L2ToL1MessagePasser)
	fmt.Printf("📍 L2ToL1MessagePasser: %s\n", messagePasserAddr.Hex())
	
	// Block number for the proof
	blockNum := big.NewInt(int64(blockNumber))
	fmt.Printf("📊 Block number: %d\n", blockNum.Uint64())
	
	// Get the block to retrieve the block hash
	block, err := m.ClientL2.HeaderByNumber(ctx, blockNum)
	if err != nil {
		return nil, fmt.Errorf("failed to get block header: %w", err)
	}
	fmt.Printf("🔗 Block hash: %s\n", block.Hash().Hex())
	
	// Calculate storage slot for sentMessages mapping
	// sentMessages[withdrawalHash] = true
	// Storage slot = keccak256(abi.encode(withdrawalHash, slot))
	// where slot = 0 for sentMessages mapping
	withdrawalHashBytes := common.HexToHash(message.WithdrawalHash)
	slot := m.calculateSentMessagesSlot(message.WithdrawalHash)
	fmt.Printf("📝 Withdrawal hash: %s\n", withdrawalHashBytes.Hex())
	fmt.Printf("📝 Storage slot: %s\n", slot.Hex())
	
	// Make eth_getProof RPC call
	type GetProofResult struct {
		AccountProof []string `json:"accountProof"`
		StorageProof []struct {
			Key   string   `json:"key"`
			Value string   `json:"value"`
			Proof []string `json:"proof"`
		} `json:"storageProof"`
		StorageHash string `json:"storageHash"`
	}
	
	var proofResult GetProofResult
	err = m.ClientL2.Client().CallContext(ctx, &proofResult, "eth_getProof", 
		messagePasserAddr.Hex(), 
		[]string{slot.Hex()}, 
		fmt.Sprintf("0x%x", blockNum.Uint64()))
	
	if err != nil {
		return nil, fmt.Errorf("failed to call eth_getProof: %w", err)
	}
	
	fmt.Printf("✅ Got proof with %d account proof elements and %d storage proof elements\n", 
		len(proofResult.AccountProof), len(proofResult.StorageProof))
	
	// Parse storage hash (this is the storage root from the account)
	storageHash := common.HexToHash(proofResult.StorageHash)
	var messagePasserStorageRoot [32]byte
	copy(messagePasserStorageRoot[:], storageHash[:])
	fmt.Printf("📊 Message Passer Storage Root: %s\n", storageHash.Hex())
	
	// The withdrawal proof should ONLY contain the storage proof, not the account proof
	// The account proof is implicitly verified through the messagePasserStorageRoot
	var withdrawalProof [][]byte
	
	// Add only storage proof elements
	if len(proofResult.StorageProof) > 0 {
		// Debug: Check the storage value
		storageValue := proofResult.StorageProof[0].Value
		fmt.Printf("📊 Storage value: %s\n", storageValue)
		if storageValue != "0x1" && storageValue != "0x01" {
			fmt.Printf("⚠️  Warning: Expected storage value 0x1 (true), got %s\n", storageValue)
		}
		
		for _, proofHex := range proofResult.StorageProof[0].Proof {
			proofBytes := common.FromHex(proofHex)
			withdrawalProof = append(withdrawalProof, proofBytes)
		}
		fmt.Printf("✅ Got storage proof with %d elements\n", len(withdrawalProof))
	} else {
		return nil, fmt.Errorf("no storage proof returned for withdrawal hash")
	}
	
	// Apply MaybeAddProofNode fix - this handles the case where the final proof element
	// is less than 32 bytes and exists inside a branch node
	var slotArray [32]byte
	copy(slotArray[:], slot[:])
	withdrawalProof, err = helper.MaybeAddProofNode(slotArray, withdrawalProof)
	if err != nil {
		return nil, fmt.Errorf("failed to apply MaybeAddProofNode: %w", err)
	}
	
	// Debug: Print proof elements in detail
	fmt.Printf("✅ Final withdrawal proof has %d elements (after MaybeAddProofNode)\n", len(withdrawalProof))
	for i, proof := range withdrawalProof {
		fmt.Printf("  Proof[%d]: %d bytes\n", i, len(proof))
		fmt.Printf("    First byte: 0x%02x (RLP prefix)\n", proof[0])
		
		// Try to determine node type from RLP structure
		var rlpData []interface{}
		err := rlp.DecodeBytes(proof, &rlpData)
		if err == nil {
			if len(rlpData) == 17 {
				fmt.Printf("    Type: Branch node (17 elements)\n")
			} else if len(rlpData) == 2 {
				fmt.Printf("    Type: Leaf/Extension node (2 elements)\n")
			} else {
				fmt.Printf("    Type: Unknown (%d elements)\n", len(rlpData))
			}
		}
		
		if len(proof) <= 64 {
			fmt.Printf("    Hex: 0x%x\n", proof)
		} else {
			fmt.Printf("    Hex (first 32): 0x%x...\n", proof[:32])
			fmt.Printf("    Hex (last 32): ...0x%x\n", proof[len(proof)-32:])
		}
	}
	
	// Get the state root from the block header
	var stateRoot [32]byte
	copy(stateRoot[:], block.Root[:])
	fmt.Printf("📊 Block State Root: %s\n", block.Root.Hex())
	
	return &WithdrawalProof{
		WithdrawalProof:          withdrawalProof,
		MessagePasserStorageRoot: messagePasserStorageRoot,
		LatestBlockhash:          block.Hash(),
		StateRoot:                stateRoot,
	}, nil
}

// calculateSentMessagesSlot calculates the storage slot for sentMessages mapping
func (m *CrossChainMessenger) calculateSentMessagesSlot(withdrawalHash string) common.Hash {
	// sentMessages mapping is at slot 0 in L2ToL1MessagePasser contract
	// Storage slot = keccak256(abi.encodePacked(withdrawalHash, mappingSlot))
	withdrawalHashBytes := common.HexToHash(withdrawalHash)
	mappingSlot := common.BigToHash(big.NewInt(0)) // sentMessages is at slot 0
	
	// Concatenate: withdrawalHash (32 bytes) + mappingSlot (32 bytes)
	data := append(withdrawalHashBytes.Bytes(), mappingSlot.Bytes()...)
	
	// Calculate keccak256
	hash := crypto.Keccak256Hash(data)
	return hash
}


// callProveWithdrawalTransaction calls the proveWithdrawalTransaction method
func (m *CrossChainMessenger) callProveWithdrawalTransaction(ctx context.Context, withdrawalTx cross_abi.TypesWithdrawalTransaction, l2OutputIndex uint64, outputRootProof cross_abi.TypesOutputRootProof, withdrawalProof [][]byte) error {
	// Create OptimismPortal contract instance
	optimismPortalAddr := common.HexToAddress(m.Contracts.L1.OptimismPortal)
	optimismPortal, err := cross_abi.NewOptimismPortal(optimismPortalAddr, m.ClientL1)
	if err != nil {
		return fmt.Errorf("failed to create OptimismPortal contract: %w", err)
	}

	// Get transaction options
	txOpts, err := m.getTransactOpts(ctx)
	if err != nil {
		return fmt.Errorf("failed to get transaction options: %w", err)
	}

	// Call proveWithdrawalTransaction
	tx, err := optimismPortal.ProveWithdrawalTransaction(
		txOpts,
		withdrawalTx,
		big.NewInt(int64(l2OutputIndex)),
		outputRootProof,
		withdrawalProof,
	)
	if err != nil {
		return fmt.Errorf("failed to prove withdrawal transaction: %w", err)
	}

	fmt.Printf("✅ Prove transaction submitted: %s\n", tx.Hash().Hex())
	
	// Print raw transaction data for manual broadcasting
	txData, err := tx.MarshalBinary()
	if err != nil {
		fmt.Printf("⚠️  Failed to marshal transaction: %v\n", err)
	} else {
		fmt.Printf("\n📦 Raw Transaction Data (for manual broadcast):\n")
		fmt.Printf("0x%x\n", txData)
		fmt.Printf("\n💡 You can broadcast this with: cast publish 0x%x --rpc-url $L1_RPC\n", txData)
	}
	
	// Wait for transaction to be mined
	fmt.Printf("\n⏳ Waiting for transaction to be mined...\n")
	receipt, err := bind.WaitMined(ctx, m.ClientL1, tx)
	if err != nil {
		return fmt.Errorf("failed to wait for transaction: %w", err)
	}
	
	if receipt.Status == 0 {
		return fmt.Errorf("transaction failed (status: 0)")
	}
	
	fmt.Printf("✅ Transaction mined in block %d (status: %d)\n", receipt.BlockNumber.Uint64(), receipt.Status)
	fmt.Printf("   Gas used: %d\n", receipt.GasUsed)
	
	return nil
}

// getTransactOpts gets transaction options for signing
func (m *CrossChainMessenger) getTransactOpts(ctx context.Context) (*bind.TransactOpts, error) {
	if m.KMSClient != nil {
		// Use KMS for signing
		return m.getKMSTransactOpts(ctx)
	} else if m.PrivateKey != "" {
		// Use private key for signing
		return m.getPrivateKeyTransactOpts()
	}
	return nil, fmt.Errorf("no signing method configured")
}

// getKMSTransactOpts gets transaction options using KMS
func (m *CrossChainMessenger) getKMSTransactOpts(ctx context.Context) (*bind.TransactOpts, error) {
	if m.KMSClient == nil {
		return nil, fmt.Errorf("KMS client not initialized")
	}

	// Get chain ID
	chainID, err := m.ClientL1.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID: %w", err)
	}

	// Use the go-ethereum-aws-kms-tx-signer library to create TransactOpts
	// This library handles all the KMS signing complexity including secp256k1 compatibility
	transactor, err := kmssigner.NewAwsKmsTransactorWithChainID(m.KMSClient, m.KMSKeyID, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create KMS transactor: %w", err)
	}

	// Set context
	transactor.Context = ctx

	return transactor, nil
}

// getPrivateKeyTransactOpts gets transaction options using private key
func (m *CrossChainMessenger) getPrivateKeyTransactOpts() (*bind.TransactOpts, error) {
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(m.PrivateKey, "0x"))
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	chainID, err := m.ClientL1.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID: %w", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactor: %w", err)
	}

	return auth, nil
}



// calculateOutputRoot calculates the output root from the output root proof
// OutputRoot = keccak256(abi.encode(version, stateRoot, messagePasserStorageRoot, latestBlockhash))
func (m *CrossChainMessenger) calculateOutputRoot(proof cross_abi.TypesOutputRootProof) [32]byte {
	// ABI encode: version (32 bytes) + stateRoot (32 bytes) + messagePasserStorageRoot (32 bytes) + latestBlockhash (32 bytes)
	data := make([]byte, 0, 128)
	data = append(data, proof.Version[:]...)
	data = append(data, proof.StateRoot[:]...)
	data = append(data, proof.MessagePasserStorageRoot[:]...)
	data = append(data, proof.LatestBlockhash[:]...)
	
	hash := crypto.Keccak256Hash(data)
	var result [32]byte
	copy(result[:], hash[:])
	return result
}