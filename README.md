# Mantle Claim Crossing Script - Go Implementation

This is a Go implementation of the Mantle cross-chain claiming script.

## Features

-   Support for both AWS KMS and private key signing
-   Cross-chain message processing
-   Ethereum client integration
-   Environment variable configuration

## Setup

1. Install Go dependencies:

```bash
go mod tidy
```

2. Copy the environment file:

```bash
cp ../.env.example .env
```

3. Configure your environment variables in `.env`:

```
# Signing Configuration (choose one)
# Option 1: Private Key (for development/testing)
#PRIV_KEY=0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef

# Option 2: AWS KMS (for production on EC2)
# KMS Key ID can be in one of the following formats:
# 1. Key ARN: arn:aws:kms:ap-northeast-1:123456789012:key/1231a8f6-de82-1234-8366-68741fd99c33
# 2. Key ID (UUID): 1231a8f6-de82-1234-8366-68741fd99c33
# 3. Alias: alias/my-key-alias

# Note: When running on EC2, AWS credentials will be automatically obtained from the IAM role
# No need to set AWS_ACCESS_KEY_ID or AWS_SECRET_ACCESS_KEY
```

For private key mode:

```
PRIV_KEY=your_private_key_here
L1_RPC=https://1rpc.io/eth
L1_CHAINID=1
L2_RPC=https://rpc.mantle.xyz
L2_CHAINID=5000
```

For KMS mode:

```
KMS_KEY_ID=1231a8f6-de82-1234-8366-68741fd99c33
AWS_REGION=ap-northeast-1
L1_RPC=https://1rpc.io/eth
L1_CHAINID=1
L2_RPC=https://rpc.mantle.xyz
L2_CHAINID=5000
```

## Usage

Run the claiming script:

```bash
go run main.go
```

## Architecture

-   **Signer Interface**: Abstraction for different signing methods (private key vs KMS)
-   **CrossChainMessenger**: Handles cross-chain operations
-   **Config**: Environment variable configuration
-   **KMSSigner**: AWS KMS integration for secure signing
-   **PrivateKeySigner**: Private key signing for development/testing

## TODO

-   [ ] Implement full cross-chain message parsing
-   [ ] Add proof generation logic
-   [ ] Implement transaction submission to L1
-   [ ] Add comprehensive error handling
-   [ ] Add logging framework
-   [ ] Add unit tests
