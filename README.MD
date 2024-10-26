# Minecraft Auth Proxy

A proxy server that allows Minecraft players to connect using unique domain names instead of passwords. Registration and management are handled through Telegram bot.

## Features
- No client modifications needed - works with vanilla Minecraft
- No server modifications required - compatible with any Minecraft server
- Easy registration through Telegram
- Secure authentication without passwords
- Multiple usernames per player
- Admin approval system for new players
- Support for Minecraft 1.19 - 1.21.2

## How it works
1. Players register through Telegram bot
2. Each player gets a unique subdomain like `abc123def456.mc.example.com`
3. Player connects to the server using this subdomain instead of password
4. Proxy validates the subdomain and forwards connection to the real Minecraft server

## Setup

1. Create a Telegram bot through [@BotFather](https://t.me/BotFather)
2. Clone this repository
3. Create `config.toml`:
```toml
Listen = "25565" # Proxy port
MinecraftServer = "25566" # Real Minecraft server port
BaseDomain = "example.com" # Your domain for player subdomains
BotToken = "123:ABC..." # Telegram bot token from BotFather
AdminID = 123456789 # Your Telegram user ID
SupportName = "@admin" # Support contact (optional)
Lang = "en" # Language: "en" or "ru"
```
4. Set up DNS:
   - `A` record for `example.com` pointing to your server
   - Wildcard record (either `A` or `CNAME`):
     - `A` record: `*.example.com` pointing to your server's IP
     - or `CNAME` record: `*.example.com` pointing to `example.com`
5. Configure your Minecraft server to listen only on localhost (127.0.0.1) to prevent direct connections
6. Run the proxy: `./minecraft-auth-proxy`

## Security Notes
- Keep your subdomain private - it's your access key
- Proxy drops connections without valid subdomain tokens - no server information is exposed
- Deleting a username through bot only frees it for registration, server data remains unchanged

## License
MIT License