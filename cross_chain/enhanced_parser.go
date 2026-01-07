package crosschain

import (
	"fmt"
	cross_abi "mantle-claim-crossing/abi"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var NonceMask, _ = new(big.Int).SetString("0000ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 16)

// parseSentMessageWithABI uses the generated ABI code to parse SentMessage events
func parseSentMessageWithABI(log *types.Log) (*cross_abi.L2CrossDomainMessengerSentMessage, error) {
	// Convert our Log structure to ethereum types.Log
	ethLog := types.Log{
		Address: log.Address,
		Topics:  make([]common.Hash, len(log.Topics)),
		Data:    log.Data,
	}
	
	// Convert topics
	copy(ethLog.Topics, log.Topics)	
	// Create a filterer with nil contract (we only need the ABI for parsing)
	filterer, _:= cross_abi.NewL2CrossDomainMessengerFilterer(common.Address{}, nil) // Just to ensure import
	
	
	// Parse using the generated ABI code
	sentMsg, err := filterer.ParseSentMessage(ethLog)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SentMessage with ABI: %w", err)
	}
	
	fmt.Printf("  📋 Parsed SentMessage (ABI): Target=%s, Sender=%s, Nonce=%s, GasLimit=%s\n", 
		sentMsg.Target.Hex(), sentMsg.Sender.Hex(), fmt.Sprintf("0x%x", sentMsg.MessageNonce), fmt.Sprintf("0x%x", sentMsg.GasLimit))
	
	return sentMsg, nil
}


// Enhanced parsing method that uses improved ABI-like parsing
func (m *CrossChainMessenger) parseSentMessageLogsEnhanced(receipt *types.Receipt) (Message, error) {
	var message Message

	// SentMessage event signature: SentMessage(address,address,bytes,uint256,uint256)
	// This is the keccak256 hash of the event signature
	sentMessageTopic := "0xcb0f7ffd78f9aee47a248fae8db181db6eee833039123e026dcbff529522e52a"

	for _, log := range receipt.Logs {
		if log.Address != common.HexToAddress(m.Contracts.Bridges.L2CrossDomainMessenger) {
			continue
		}
		// fmt.Printf("📄 Log %d: address=%s, topics=%v\n", i, log.Address, log.Topics)
		// fmt.Printf("  📝 Raw log data: %s\n", hex.EncodeToString(log.Data))
		
		// Parse block number and log index
		blockNumber := receipt.BlockNumber.Uint64()
		logIndex := uint64(log.Index)
		// Try to parse using the generated ABI code first (BEST METHOD)
		if len(log.Topics) > 0 && strings.EqualFold(log.Topics[0].String(), sentMessageTopic) {
			eventData, _ := parseSentMessageWithABI(log)

			message = Message{
				TxHash:      receipt.TxHash.Hex(),
				BlockNumber: blockNumber,
				LogIndex:    logIndex,
				Direction:   "L2_TO_L1",
				Status:      0, // Will be updated later
				SentMessageEvent:   eventData,
			}
		}
	}

	return message, nil
}

func (m *CrossChainMessenger) parseSentMessageExtension1LogsEnhanced(receipt *types.Receipt) (*cross_abi.L2CrossDomainMessengerSentMessageExtension1, error) {
	var messagePassed *cross_abi.L2CrossDomainMessengerSentMessageExtension1

	sentMessageExtension1Topic := "0xcf00802ba1f8c659140235227979ca08afaba336a9f9fdc4a5107ed9e8013d08"

	for _, log := range receipt.Logs {
		if log.Address != common.HexToAddress(m.Contracts.Bridges.L2ToL1MessagePasser) {
			continue
		}
		// fmt.Printf("📄 Log %d: address=%s, topics=%v\n", i, log.Address, log.Topics)
		// fmt.Printf("  📝 Raw log data: %s\n", hex.EncodeToString(log.Data))
		
		// Try to parse using the generated ABI code first (BEST METHOD)
		if len(log.Topics) > 0 && strings.EqualFold(log.Topics[0].String(), sentMessageExtension1Topic) {
			messagePassed, _ = parseSentMessageExtension1WithABI(log)
		}
	}

	return messagePassed, nil
}

func parseSentMessageExtension1WithABI(log *types.Log) (*cross_abi.L2CrossDomainMessengerSentMessageExtension1, error) {
	// Convert our Log structure to ethereum types.Log
	ethLog := types.Log{
		Address: log.Address,
		Topics:  make([]common.Hash, len(log.Topics)),
		Data:    log.Data,
	}
	
	// Convert topics
	copy(ethLog.Topics, log.Topics)	
	// Create a filterer with nil contract (we only need the ABI for parsing)
	filterer, _:= cross_abi.NewL2CrossDomainMessengerFilterer(common.Address{}, nil) // Just to ensure import
	
	
	// Parse using the generated ABI code
	sentMsg, err := filterer.ParseSentMessageExtension1(ethLog)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SentMessage with ABI: %w", err)
	}
	
	fmt.Printf("  📋 Parsed SentMessage (ABI): Sender=%s, MntValue=%s, EthValue=%s\n", 
		sentMsg.Sender.Hex(), sentMsg.MntValue.String(), sentMsg.EthValue.String())
	
	return sentMsg, nil
}

func (m *CrossChainMessenger) parseMessagePassedLogsEnhanced(receipt *types.Receipt) (*cross_abi.L2ToL1MessagePasserMessagePassed, error) {
	var messagePassed *cross_abi.L2ToL1MessagePasserMessagePassed

	messagePassedTopic := "0x5da382596b838a63b4248e533d8e399b3b0f13ba6c6679f670489d44716cb173"

	for _, log := range receipt.Logs {
		if log.Address != common.HexToAddress(m.Contracts.Bridges.L2ToL1MessagePasser) {
			continue
		}
		// fmt.Printf("📄 Log %d: address=%s, topics=%v\n", i, log.Address, log.Topics)
		// fmt.Printf("  📝 Raw log data: %s\n", hex.EncodeToString(log.Data))
		
		// Try to parse using the generated ABI code first (BEST METHOD)
		if len(log.Topics) > 0 && strings.EqualFold(log.Topics[0].String(), messagePassedTopic) {
			messagePassed, _ = parseMessagePassedWithABI(log)
		}
	}

	return messagePassed, nil
}

func parseMessagePassedWithABI(log *types.Log) (*cross_abi.L2ToL1MessagePasserMessagePassed, error) {
	// Convert our Log structure to ethereum types.Log
	ethLog := types.Log{
		Address: log.Address,
		Topics:  make([]common.Hash, len(log.Topics)),
		Data:    log.Data,
	}
	
	// Convert topics
	copy(ethLog.Topics, log.Topics)	
	// Create a filterer with nil contract (we only need the ABI for parsing)
	filterer, _:= cross_abi.NewL2ToL1MessagePasserFilterer(common.Address{}, nil) // Just to ensure import
	
	// Parse using the generated ABI code
	messagePassed, err := filterer.ParseMessagePassed(ethLog)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MessagePassed with ABI: %w", err)
	}
	
	// fmt.Printf("  📋 Parsed MessagePassed (ABI): Target=%s, Sender=%s, Data=%s, Nonce=%s, GasLimit=%s, WithdrawHash=%s\n", 
	// 	messagePassed.Target.Hex(), messagePassed.Sender.Hex(), hex.EncodeToString(messagePassed.Data[:]), messagePassed.Nonce.String(), messagePassed.GasLimit.String(), hex.EncodeToString(messagePassed.WithdrawalHash[:]))
	
	return messagePassed, nil
}

