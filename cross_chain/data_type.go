package crosschain

import (
	"encoding/json"
	cross_abi "mantle-claim-crossing/abi"
	"math/big"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/ethereum/go-ethereum/ethclient"
)

// CrossChainMessenger handles cross-chain operations
type CrossChainMessenger struct {
	L1RpcUrl      string
	L2RpcUrl      string
	KMSKeyID      string      // AWS KMS key ID for signing (if using KMS)
	KMSClient     *kms.Client // AWS KMS Client
	PrivateKey    string      // Private key hex for signing (if not using KMS)
	WalletAddress string
	ClientL1      *ethclient.Client
	ClientL2      *ethclient.Client
	Contracts     CrossChainContracts
}

type CrossChainContracts struct {
	L1 L1Contracts
	Bridges BridgeContracts
}

type L1Contracts struct {
	StateCommitmentChain   string
	CanonicalTransactionChain string
	BondManager            string
	AddressManager         string
	L1CrossDomainMessenger string
	L1StandardBridge       string
	OptimismPortal         string
	L2OutputOracle         string
}

type BridgeContracts struct {
	L1Bridge string
	L2Bridge string
	Adapter  string
	L2CrossDomainMessenger string
	L2ToL1MessagePasser string
}



// Message represents a cross-chain message
type Message struct {
	TxHash      string
	BlockNumber uint64
	LogIndex    uint64
	Direction   string
	Status      int
	MsgNonce *big.Int
	WithdrawalHash string
	MntValue *big.Int
	EthValue *big.Int
	SentMessageEvent   *cross_abi.L2CrossDomainMessengerSentMessage
	SentMessageExtension1Event *cross_abi.L2CrossDomainMessengerSentMessageExtension1
	MessagePassedEvent *cross_abi.L2ToL1MessagePasserMessagePassed
}

// RPCRequest represents a JSON-RPC request
type RPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

// RPCResponse represents a JSON-RPC response
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}



// WithdrawalProof represents the proof data for a withdrawal
type WithdrawalProof struct {
	WithdrawalProof          [][]byte
	MessagePasserStorageRoot [32]byte
	LatestBlockhash          [32]byte
	StateRoot                [32]byte
}



// DERSignature represents a DER-encoded signature
type DERSignature struct {
	R *big.Int
	S *big.Int
}

// EthereumSignature represents an Ethereum-compatible signature
type EthereumSignature struct {
	R string `json:"r"`
	S string `json:"s"`
	V int    `json:"v"`
}