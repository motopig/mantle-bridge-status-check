package crosschain

import (
	"encoding/asn1"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Helper functions

// parseHexToUint64 converts hex string to uint64
func parseHexToUint64(hexStr string) (uint64, error) {
	hexStr = strings.TrimPrefix(hexStr, "0x")
	return strconv.ParseUint(hexStr, 16, 64)
}

// getStatusDescription returns human-readable status description
func getStatusDescription(status int) string {
	switch status {
	case 0:
		return "READY_TO_PROVE"
	case 1:
		return "PROVEN"
	case 2:
		return "RELAYED/FINALIZED"
	default:
		return "UNKNOWN"
	}
}

// parseDERSignature parses a DER-encoded signature
func parseDERSignature(derBytes []byte) (*DERSignature, error) {
	var sig DERSignature
	_, err := asn1.Unmarshal(derBytes, &sig)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal DER signature: %w", err)
	}
	return &sig, nil
}

// getEnvOrDefault gets environment variable with default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getCurrentTimestamp returns current Unix timestamp
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}

