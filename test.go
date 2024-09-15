package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

func test(client *ethclient.Client, sql *sql.DB) {
	blockNumber := big.NewInt(20745417)
	block, err := client.BlockByNumber(context.Background(), blockNumber)
	if err != nil {
		fmt.Printf("Failed to retrieve block %v ", err)
		return
	}
	const minimumValue = 100000000000000000

	tokenList := [][]interface{}{}
	rows, err := sql.Query("SELECT id, deployer, contract_addr from token_info ")
	if err != nil {
		fmt.Printf("Error reading database")
	}
	defer rows.Close()
	// fmt.Printf("Type: %T\n", rows)
	for rows.Next() {
		var deployer, contract_addr string
		var id int
		rows.Scan(&id, &deployer, &contract_addr)
		// fmt.Printf("id: %v deployer %v contract %v\n", id, deployer, contract_addr)
		tokenList = append(tokenList, []interface{}{id, deployer, contract_addr})
	}

	for _, tx := range block.Transactions() {
		deployer, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
		if err != nil {
			fmt.Printf("Failed to retrieve sender: %v\n", err)
			continue
		}
		var universalRouter = common.HexToAddress("0x3fC91A3afd70395Cd496C647d5a6CC9D4B2b7FAD")
		if tx.To() != nil && tx.To().Cmp(universalRouter) == 0 && tx.Value().Cmp(big.NewInt(minimumValue)) > 0 {
			// fmt.Printf("I am In\n")
			// Check if the input data contains the token address (tk_addr)
			ind := searchFromDbTokens(tokenList, tx, deployer)
			if ind != -1 {
				fmt.Printf("Transaction %s matches criteria\n", tx.Hash())
			}
		}
	}
}
