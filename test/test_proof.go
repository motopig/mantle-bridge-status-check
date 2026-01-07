package main

import (
	"encoding/hex"
	"fmt"

	"github.com/ethereum/go-ethereum/rlp"
)

func main() {
	// Test with a sample proof element from the debug output
	// This is a typical branch node from eth_getProof
	proofHex := "f90211a0e3f7c2f33a4e1b7c6d5e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0a1a0e4f8c3f44a5e2b8c7d6e9f0b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1a2a0e5f9c4f55a6e3b9c8d7e0f1b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2a3a0e6fac5f66a7e4bac9d8e1f2b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9fab1c3a4a0e7fbc6f77a8e5bbcad9e2f3b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4a5a0e8fcc7f88a9e6cbdbae3f4b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5a6a0e9fdc8f99aae7dcebcf4f5b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6a7a0eafed9faabbefeecfedfef5ebfc8de9efa1fb2c3d4e5f6a7b8c9d0e1f2a3b4c5a6a0ebffaeafbbcffdfeffeffefffc9efaeff2b3c4d5e6f7a8b9c0d1e2f3a4b5c6a7a0ecfff0bffcccfeffffeffeffeffdfebff3c4d5e6f7a8b9c0d1e2f3a4b5c6a7a8a0edfff1cffdddfffefffffffffefffcfff4d5e6f7a8b9c0d1e2f3a4b5c6a7a8a9a0eefff2dffeeeffffffffffffffffffffe6f7a8b9c0d1e2f3a4b5c6a7a8a9aaa0effff3effeffffffffffffffffffffffffffa8b9c0d1e2f3a4b5c6a7a8a9aabba0f0fff4fffeffffffffffffffffffffffffffffffffffc0d1e2f3a4b5c6a7a8a9aabbcca0f1fff5fffeffffffffffffffffffffffffffffffffffffffe2f3a4b5c6a7a8a9aabbccdda0f2fff6fffeffffffffffffffffffffffffffffffffffffffffffa4b5c6a7a8a9aabbccddee80"
	
	proofBytes, err := hex.DecodeString(proofHex)
	if err != nil {
		fmt.Printf("Failed to decode hex: %v\n", err)
		return
	}
	
	fmt.Printf("Proof element length: %d bytes\n", len(proofBytes))
	fmt.Printf("First few bytes: %x\n", proofBytes[:10])
	
	// Try to decode as RLP
	var decoded []interface{}
	err = rlp.DecodeBytes(proofBytes, &decoded)
	if err != nil {
		fmt.Printf("Failed to decode RLP: %v\n", err)
		return
	}
	
	fmt.Printf("Decoded RLP has %d elements\n", len(decoded))
	
	// Check if it's a branch node (17 elements)
	if len(decoded) == 17 {
		fmt.Println("✅ This is a branch node (17 elements)")
	} else if len(decoded) == 2 {
		fmt.Println("✅ This is a leaf or extension node (2 elements)")
	} else {
		fmt.Printf("⚠️  Unexpected node type with %d elements\n", len(decoded))
	}
}
