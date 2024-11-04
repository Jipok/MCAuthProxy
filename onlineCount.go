package main

import (
	"log"
	"net"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

const (
	OfflineCheckInterval = time.Second * 2 // Частая проверка когда офлайн
	OnlineCheckInterval  = time.Minute * 5 // Редкая проверка когда онлайн но пусто
)

var (
	onlinePlayers = struct {
		sync.RWMutex
		players map[string]bool
	}{
		players: make(map[string]bool),
	}

	serverStatus struct {
		sync.RWMutex
		isOnline  bool
		lastCheck time.Time
	}
)

func addPlayer(nickname string) {
	onlinePlayers.Lock()
	onlinePlayers.players[nickname] = true
	onlinePlayers.Unlock()
}

func removePlayer(nickname string) {
	onlinePlayers.Lock()
	delete(onlinePlayers.players, nickname)
	if len(onlinePlayers.players) == 0 {
		go updateServerStatus()
	}
	onlinePlayers.Unlock()
}

func getOnlinePlayers() []string {
	onlinePlayers.RLock()
	defer onlinePlayers.RUnlock()

	players := make([]string, 0, len(onlinePlayers.players))
	for player := range onlinePlayers.players {
		players = append(players, player)
	}
	return players
}

///////////////////////////////////////////////////////////////////////////////

func updateOnlineMessage() {
	if cfg.OnlineMessageID == 0 {
		return
	}

	players := getOnlinePlayers()
	var msg string

	if len(players) == 0 {
		msg = "Online: 0"
	} else {
		msg = "Online: "
		for i, player := range players {
			if i > 0 {
				msg += ", "
			}
			msg += player
		}
	}

	if !serverStatus.isOnline {
		msg = "Offline"
	}

	// TODO 20 msg per minute in groups?
	_, _, err := bot.EditMessageText(msg, &gotgbot.EditMessageTextOpts{
		ChatId:    cfg.OnlineMessageChatID,
		MessageId: cfg.OnlineMessageID,
	})
	if err != nil {
		log.Printf("Failed to update online message: %v", err)
	}
}

///////////////////////////////////////////////////////////////////////////////

func updateServerStatus() {
	dialer := net.Dialer{
		Timeout: DialTimeout,
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(ProxyBind),
		},
	}

	currentStatus := false
	conn, err := dialer.Dial("tcp", cfg.MinecraftServer)
	if err == nil {
		conn.Close()
		currentStatus = true
	}

	serverStatus.Lock()
	defer serverStatus.Unlock()

	if serverStatus.isOnline != currentStatus {
		serverStatus.isOnline = currentStatus
		serverStatus.lastCheck = time.Now()

		if currentStatus {
			log.Println("Server is now ONLINE")
			bot.SendMessage(cfg.AdminID, "🟢 Server is online.", nil)
		} else {
			log.Println("Server is now OFFLINE")
			bot.SendMessage(cfg.AdminID, "🔴 Server is now OFFLINE!", nil)
		}
		updateOnlineMessage()
	}
}

func startServerStatusChecker() {
	ticker := time.NewTicker(OfflineCheckInterval)
	defer ticker.Stop()

	for {
		if serverStatus.isOnline {
			if time.Since(serverStatus.lastCheck) > OnlineCheckInterval {
				updateServerStatus()
			}
		} else {
			updateServerStatus()
		}

		<-ticker.C
	}
}
