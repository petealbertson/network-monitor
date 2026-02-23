# Network Monitor

Simple network uptime monitor with Telegram notifications.

## Features

- Pings a target host every 5 minutes (configurable)
- Sends Telegram notification only on status changes (up→down or down→up)
- Telegram bot commands:
  - `/status` - Check current status
  - `/ping` - Force an immediate check
  - `/start` - Shows your chat ID

## Setup

### 1. Create a Telegram Bot

- Message **@BotFather** on Telegram
- Send `/newbot` and follow the prompts
- Copy the bot token

### 2. Get Your Chat ID

Message your new bot, then run:
```bash
curl "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/getUpdates"
```
Look for `"chat":{"id":XXXXXXX}`

### 3. Configure

```bash
cp config.json.example config.json
```

Edit `config.json`:
```json
{
  "target": "your-home-ip-or-hostname",
  "bot_token": "your-telegram-bot-token",
  "chat_id": "your-telegram-chat-id",
  "ping_interval": 300
}
```

### 4. Build & Run

```bash
go build -o network-monitor .
./network-monitor
```

### 5. Install as systemd service (optional)

```bash
sudo cp network-monitor.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable network-monitor
sudo systemctl start network-monitor
```

Check logs: `journalctl -u network-monitor -f`
