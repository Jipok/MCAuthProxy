package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Listen              string
	MinecraftServer     string
	BaseDomain          string
	BotToken            string
	AdminID             int64
	OnlineMessageID     int64
	OnlineMessageChatID int64
	SupportName         string
	Lang                string
}

var (
	storage    *Storage
	cfg        Config
	configFile string
)

func SaveConfig(config Config) {
	file, err := os.Create(configFile)
	if err != nil {
		log.Printf("failed to create config file: %s", err)
		return
	}
	defer file.Close()

	encoder := toml.NewEncoder(file)
	if err := encoder.Encode(config); err != nil {
		log.Printf("failed to encode config: %s", err)
		return
	}
}

func isValidMinecraftUsername(username string) bool {
	if username == "online" || username == "list" || username == "delete" {
		return false
	}
	if len(username) < 3 || len(username) > 16 {
		return false
	}
	match, _ := regexp.MatchString(`^[A-Za-z0-9_]+$`, username)
	return match
}

func getUserInfoByHostname(host string) *StorageRecord {
	// Remove port if present
	parts := strings.SplitN(host, ":", 2)
	host = parts[0]

	// Check server address
	token, correctDomain := strings.CutSuffix(host, "."+cfg.BaseDomain)
	if !correctDomain {
		log.Printf("Someone tried to connect using address: %s\n", host)
		return nil
	}

	// Check token
	userInfo, err := storage.FindByToken(token)
	if err != nil {
		log.Printf("Someone tried to connect using bad token: %s\n", host)
		return nil
	}
	return userInfo
}

func main() {
	configFile = "./config.toml"
	if len(os.Args) > 1 {
		configFile = os.Args[1]
	}
	_, err := toml.DecodeFile(configFile, &cfg)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Fatalf("Configuration file `%s` not found", configFile)
		} else {
			log.Fatal(err)
		}
	}

	//
	if !strings.Contains(cfg.Listen, ":") {
		cfg.Listen = "0.0.0.0:" + cfg.Listen
	}
	if !strings.Contains(cfg.MinecraftServer, ":") {
		cfg.MinecraftServer = "127.0.0.1:" + cfg.MinecraftServer
	}

	storage = NewStorage("data.txt")
	go startMinecraftProxy()
	updater := startTgBot()

	// Add telegram users to bot access
	usersInfo, err := storage.readRecords()
	if err != nil {
		log.Fatal(err)
	}
	for _, userInfo := range usersInfo {
		allowedIDs.Add(int64(userInfo.ID))
	}
	// And admin too
	allowedIDs.Add(cfg.AdminID)

	updateOnlineMessage()
	go startServerStatusChecker()

	// Handling Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	go func() {
		<-sigChan
		fmt.Println("Received SIGINT, stopping bot...")
		updater.Stop()
	}()
	updater.Idle()
}
