package main

import (
	"bufio"
	"os"
	"strings"

	bot "github.com/mohamed/cf-discord-bot/bot"
)

func loadDotEnv(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"`)
		_ = os.Setenv(key, value)
	}
}

func main() {
	loadDotEnv(".env")
	bot.BotToken = os.Getenv("BOT_TOKEN")
	bot.Run()
}
