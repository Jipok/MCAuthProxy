# Minecraft Auth Proxy

A proxy server that allows Minecraft players to connect using unique domain names instead of passwords. Registration and management are handled through Telegram bot.
No more typing /login <password> every time. Just add the server to the server list and connect.

![screenshot](/screenshot.png?raw=true)

## Features
- No client modifications needed - works with vanilla Minecraft
- No server modifications required - compatible with any Minecraft server
- Easy registration through Telegram
- Secure authentication without passwords
- Multiple usernames per player
- Admin approval system for new players
- Full resource pack support - seamlessly proxies resource pack downloads
- Compatible with all Minecraft versions
- Works with voice chat mods (like Plasmo Voice) that use UDP traffic on the same port

## How it works
1. Players register through Telegram bot
2. Each player gets a unique subdomain like `r2Gxb6mZWkRNQCbc54HP.example.com`
3. Player connects to the server using this subdomain instead of password
4. Proxy validates the subdomain and forwards connection to the real Minecraft server

## Setup

1. Create a Telegram bot through [@BotFather](https://t.me/BotFather)
2. Download binary from [Releases page](https://github.com/Jipok/MCAuthProxy/releases/latest)
3. Create `config.toml`:
```toml
Listen = "25565" # Proxy port
MinecraftServer = "25566" # Real Minecraft server port
BaseDomain = "example.com" # Your domain for player subdomains
BotToken = "123:ABC..." # Telegram bot token from BotFather
AdminID = 123456789 # Your Telegram user ID
SupportName = "@admin" # Support contact
Lang = "en" # Language: "en" or "ru"
```
4. Set up DNS:
   - **A** record for `example.com` pointing to your server
   - Wildcard record (either **A** or **CNAME**):
     - **A** record: `*.example.com` pointing to your server's IP
     - or **CNAME** record: `*.example.com` pointing to `example.com`
5. Configure your Minecraft server to listen only on localhost (127.0.0.1) to prevent direct connections
6. Run the proxy: `./minecraft-auth-proxy`

## Security Notes
- Keep your subdomain private - it's your access key
- Firewall: Ensure your real Minecraft server port (25566 in the example) is blocked by your firewall from public access. Only the proxy port (25565) should be open.
- Proxy drops connections without valid subdomain tokens - no server information is exposed
- Deleting a username through bot only frees it for registration, server data remains unchanged

## License
MIT License