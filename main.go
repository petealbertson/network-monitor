package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Target       string `json:"target"`        // IP or hostname to ping
	BotToken     string `json:"bot_token"`     // Telegram bot token
	ChatID       string `json:"chat_id"`       // Telegram chat ID
	PingInterval int    `json:"ping_interval"` // Interval in seconds (default 300 = 5min)
}

type Monitor struct {
	config     Config
	isUp       bool
	lastChange time.Time
	lastCheck  time.Time
	mu         sync.RWMutex
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return Config{}, err
	}
	if cfg.PingInterval == 0 {
		cfg.PingInterval = 300 // default 5 minutes
	}
	return cfg, nil
}

func (m *Monitor) ping() bool {
	// Check if target is a URL (HTTP check) or host (ICMP ping)
	if strings.HasPrefix(m.config.Target, "http://") || strings.HasPrefix(m.config.Target, "https://") {
		return m.httpCheck()
	}
	// Use ping command with 3 attempts, 2 second timeout each
	cmd := exec.Command("ping", "-c", "3", "-W", "2", m.config.Target)
	err := cmd.Run()
	return err == nil
}

func (m *Monitor) httpCheck() bool {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(m.config.Target)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	// Any response (even 401, 403) means the server is reachable
	return resp.StatusCode > 0
}

func (m *Monitor) sendTelegram(message string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", m.config.BotToken)
	resp, err := http.PostForm(apiURL, url.Values{
		"chat_id": {m.config.ChatID},
		"text":    {message},
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}
	return nil
}

func (m *Monitor) check() {
	isUp := m.ping()
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastCheck = now
	prevUp := m.isUp
	firstCheck := m.lastChange.IsZero()

	if isUp != prevUp || firstCheck {
		m.isUp = isUp
		m.lastChange = now

		var msg string
		if isUp {
			if firstCheck {
				msg = fmt.Sprintf("🟢 Network monitor started. %s is UP.", m.config.Target)
			} else {
				downtime := now.Sub(m.lastChange).Round(time.Second)
				msg = fmt.Sprintf("🟢 %s is back UP! (was down for %s)", m.config.Target, downtime)
			}
		} else {
			if firstCheck {
				msg = fmt.Sprintf("🔴 Network monitor started. %s is DOWN!", m.config.Target)
			} else {
				msg = fmt.Sprintf("🔴 %s is DOWN!", m.config.Target)
			}
		}

		log.Printf("Status change: %s", msg)
		if err := m.sendTelegram(msg); err != nil {
			log.Printf("Failed to send Telegram message: %v", err)
		}
	} else {
		log.Printf("Ping %s: still %s", m.config.Target, map[bool]string{true: "UP", false: "DOWN"}[isUp])
	}
}

func (m *Monitor) getStatus() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := "UP 🟢"
	if !m.isUp {
		status = "DOWN 🔴"
	}

	duration := time.Since(m.lastChange).Round(time.Second)
	lastCheckAgo := time.Since(m.lastCheck).Round(time.Second)

	return fmt.Sprintf("Target: %s\nStatus: %s\nSince: %s ago\nLast check: %s ago",
		m.config.Target, status, duration, lastCheckAgo)
}

func (m *Monitor) runBot() {
	offset := 0
	for {
		updates, err := m.getUpdates(offset)
		if err != nil {
			log.Printf("Error getting updates: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, update := range updates {
			offset = update.UpdateID + 1
			if update.Message == nil {
				continue
			}

			text := strings.TrimSpace(update.Message.Text)
			chatID := fmt.Sprintf("%d", update.Message.Chat.ID)

			log.Printf("Received message from chat %s: %s", chatID, text)

			if strings.HasPrefix(text, "/status") {
				m.replyToChat(chatID, m.getStatus())
			} else if strings.HasPrefix(text, "/start") {
				m.replyToChat(chatID, fmt.Sprintf("Network Monitor Bot\n\nYour chat ID: %s\n\nCommands:\n/status - Check current status", chatID))
			} else if strings.HasPrefix(text, "/ping") {
				m.replyToChat(chatID, "Checking now...")
				go func() {
					m.check()
					m.replyToChat(chatID, m.getStatus())
				}()
			}
		}

		time.Sleep(1 * time.Second)
	}
}

type TelegramUpdate struct {
	UpdateID int `json:"update_id"`
	Message  *struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

func (m *Monitor) getUpdates(offset int) ([]TelegramUpdate, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", m.config.BotToken, offset)
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool             `json:"ok"`
		Result []TelegramUpdate `json:"result"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}
	return result.Result, nil
}

func (m *Monitor) replyToChat(chatID, message string) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", m.config.BotToken)
	_, err := http.PostForm(apiURL, url.Values{
		"chat_id": {chatID},
		"text":    {message},
	})
	if err != nil {
		log.Printf("Failed to reply: %v", err)
	}
}

func main() {
	configPath := "config.json"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	config, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Starting network monitor for %s (interval: %ds)", config.Target, config.PingInterval)

	monitor := &Monitor{config: config}

	// Initial check
	monitor.check()

	// Start the Telegram bot listener
	go monitor.runBot()

	// Periodic pings
	ticker := time.NewTicker(time.Duration(config.PingInterval) * time.Second)
	for range ticker.C {
		monitor.check()
	}
}
