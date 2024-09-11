package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"uniswaptgbot/config"
	"uniswaptgbot/erc20"

	"database/sql"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	_ "github.com/go-sql-driver/mysql"
)

func main() {
	nodeUrl := config.Config("ETHEREUM_NODE_URL")
	dbUrl := config.Config("DB_URL")
	fmt.Println(nodeUrl)
	fmt.Println(dbUrl)
	sql, err := sql.Open("mysql", dbUrl)
	if err != nil {
		panic(err)
	}
	fmt.Println("Connected to database successfully.")

	client, err := ethclient.Dial(nodeUrl)
	if err != nil {
		panic(err)
	}
	headers := make(chan *types.Header)
	sub, err := client.SubscribeNewHead(context.Background(), headers)
	if err != nil {
		fmt.Printf("Failed to subscribe to new head: %v\n", err)
	}

	//monitor new blocks
	for {
		select {
		case err := <-sub.Err():
			fmt.Printf("Subscription Error %v!", err)
		case header := <-headers:
			block, err := client.BlockByHash(context.Background(), header.Hash())
			if err != nil {
				fmt.Printf("Failed to retrieve block %v ", err)
				break
			}
			// Process each transaction in the block
			for _, tx := range block.Transactions() {
				if tx.To() == nil {
					deployer, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
					if err != nil {
						fmt.Printf("Failed to retrieve sender: %v\n", err)
						continue
					}
					contractAddr := crypto.CreateAddress(deployer, tx.Nonce())
					deployer_balance, err := client.BalanceAt(context.Background(), deployer, nil)
					if err != nil {
						fmt.Printf("Failed to retrieve deployer balance: %v\n", err)
						continue
					}
					//Check wheter it's ERC20 token
					bERC20 := isERC20(contractAddr, client)
					if bERC20 {
						// Get token information
						fmt.Println("New Token Deployed!")
						fmt.Printf("Deployer Address: %s\n", deployer.Hex())
						fmt.Printf("Deployer Balance: %s\n", deployer_balance.String())
						fmt.Printf("Contract Address: %s\n", contractAddr.Hex())
						name, totSupply, decimals, symbol, err := getTokenInfo(contractAddr, client)
						if err != nil {
							fmt.Printf("Error getting token info: %s\n", err)
							continue
						}
						funded_by, fund_amount := getFundInfo(deployer, client)
						fmt.Printf("funded_by: %s fund_amount: %s", funded_by, fund_amount)

						fmt.Printf("Token Name: %s", name)
						fmt.Printf("Total Supply: %s", totSupply.String())
						sql.Query("INSERT INTO ethereum (name, total_supply, symbol, decimals, deployer, deployer_balance, funded_by, fund_amount) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
							name, totSupply.String(), symbol, decimals, deployer.Hex(), deployer_balance.String(), funded_by, fund_amount)
					}
				}
			}
		}
	}
}
func getFundInfo(creatorAddr common.Address, client *ethclient.Client) (string, string) {
	header, err := client.HeaderByNumber(context.Background(), nil)
	if err != nil {
		fmt.Printf("Failed to get latest block header: %v", err)
		return "", "0"
	}
	latestBlock := header.Number
	startBlock := big.NewInt(0) // Start from the genesis block
	endBlock := latestBlock     // Use the latest block number
	fmt.Printf("latestBlock: %v\n", latestBlock)
	fmt.Printf("startblock: %v\n", startBlock)
	query := ethereum.FilterQuery{
		FromBlock: startBlock,
		ToBlock:   endBlock,
		Addresses: []common.Address{creatorAddr},
	}
	contractABI, err := abi.JSON(strings.NewReader(`[{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address","name":"from","type":"address"},{"indexed":true,"internalType":"address","name":"to","type":"address"},{"indexed":false,"internalType":"uint256","name":"value","type":"uint256"}],"name":"Transfer","type":"event"}]`))
	if err != nil {
		fmt.Printf("Failed to load abi %v", err)
	}
	logs, err := client.FilterLogs(context.Background(), query)
	fmt.Printf("log count: %v\n", len(logs))
	if err != nil {
		fmt.Printf("Failed to filter logs %v", err)
		return "", "0"
	}
	for index := len(logs) - 1; index >= 0; index-- {
		vLog := logs[index]
		fmt.Println("Transaction Hash:", vLog.TxHash.Hex())
		fmt.Println("Block Number:", vLog.BlockNumber)
		fmt.Println("Index in Block:", vLog.Index)
		fmt.Println("Data:", vLog.Data)     // This is the raw data emitted by the event
		fmt.Println("Topics:", vLog.Topics) // Event signature and indexed parameters

		var transferEvent struct {
			From  common.Address
			To    common.Address
			Value *big.Int
		}
		err := contractABI.UnpackIntoInterface(&transferEvent, "Transfer", vLog.Data)
		if err != nil {
			fmt.Printf("Failed to unpack log: %v", err)
			return "", "0"
		}

		// Check if the log has an "amount" (in this case, the `Value` field)
		if transferEvent.Value != nil {
			fmt.Printf("Amount: %s\n", transferEvent.Value.String())
			fmt.Printf("From: %s\n", transferEvent.From.Hex())
			fmt.Printf("To: %s\n", transferEvent.To.Hex())
			fmt.Printf("Transaction Hash: %s\n", vLog.TxHash.Hex())
			bErc20 := isERC20(transferEvent.From, client)

			if bErc20 {
				name, _, _, _, err := getTokenInfo(transferEvent.From, client)
				if err != nil {
					fmt.Printf("An error occured while getting token name %v", err)
					name = ""
				}
				return name, transferEvent.Value.String()
			}

			return transferEvent.From.Hex(), transferEvent.Value.String()
		}
	}
	return "", "0"
}
func isERC20(contractAddr common.Address, client *ethclient.Client) bool {
	code, err := client.CodeAt(context.Background(), contractAddr, nil)
	if err != nil {
		fmt.Printf("Failed to retrieve contract code: %v", err)
	}
	if len(code) == 0 {
		fmt.Printf("no contract code at given address")
		return false
	}

	hexCode := hex.EncodeToString(code)

	var erc20Signatures = []string{
		"18160ddd", // totalSupply()
		"70a08231", // balanceOf(address)
		"a9059cbb", // transfer(address,uint256)
		"23b872dd", // transferFrom(address,address,uint256)
		"095ea7b3", // approve(address,uint256)
		"dd62ed3e", // allowance(address,address)
	}

	for _, sig := range erc20Signatures {
		if !strings.Contains(hexCode, sig) {
			return false
		}
	}

	return true
}

func getTokenInfo(contractAddr common.Address, client *ethclient.Client) (string, *big.Int, uint8, string, error) {
	instance, err := erc20.NewGGToken(contractAddr, client)
	if err != nil {
		fmt.Printf("Failed to instantiate contract: %v\n", err)
		return "", nil, 0, "", err
	}
	//Get token name
	name, err := instance.Name(&bind.CallOpts{})
	if err != nil {
		fmt.Printf("Failed to retrieve token name: %v\n", err)
		return "", nil, 0, "", err
	}
	fmt.Printf("Token Name: %s\n", name)
	//Get total Supply
	totalSupply, err := instance.TotalSupply(&bind.CallOpts{})
	if err != nil {
		fmt.Printf("Failed to retrieve total supply: %v\n", err)
		return "", nil, 0, "", err
	}
	fmt.Printf("Total Supply: %s\n", totalSupply.String())
	//Get decimals
	decimals, err := instance.Decimals(&bind.CallOpts{})
	if err != nil {
		fmt.Printf("Failed to retrieve decimals: %v\n", err)
		return "", nil, 0, "", err
	}
	fmt.Printf("Decimals: %v\n", decimals)
	//Get symbol
	symbol, err := instance.Symbol(&bind.CallOpts{})
	if err != nil {
		fmt.Printf("Failed to retrieve symbol: %v\n", err)
		return "", nil, 0, "", err
	}
	fmt.Printf("Symbol: %s\n", symbol)
	return name, totalSupply, decimals, symbol, nil
}