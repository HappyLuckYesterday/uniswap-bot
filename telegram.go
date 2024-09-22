package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"uniswaptgbot/config"
)

func sendMessage(botToken, chatID, text string) {
	url := "https://api.telegram.org/bot" + botToken + "/sendMessage"
	message := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}
	bytesRepresentation, err := json.Marshal(message)
	if err != nil {
		panic(err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(bytesRepresentation))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
}

func postDeployerTrans(txt string) {
	chatID := config.Config("TG_CHANNEL_ID")
	botToken := config.Config("TELEGRAM_BOT_TOKEN")
	sendMessage(botToken, chatID, txt)
}
