package main

import (
	"bufio"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	NetDeadline = time.Second * 5
	DialTimeout = time.Second * 5
	ProxyBind   = ""
)

func startMinecraftProxy() {
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
	defer listener.Close()

	log.Println("Proxy server listening on ", cfg.Listen)

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		go handleConnection(clientConn)
	}
}

func handleConnection(clientConn net.Conn) {
	reader := bufio.NewReader(clientConn)

	// Set a deadline to prevent hanging while waiting for data
	clientConn.SetDeadline(time.Now().Add(NetDeadline))

	// Try to read the first line as an HTTP request
	firstLine, err := reader.Peek(7) // GET или HTTP
	if err == nil && (strings.HasPrefix(string(firstLine), "GET") || strings.HasPrefix(string(firstLine), "HTTP")) {
		handleResourcePackRequest(clientConn, reader)
		return
	}

	// Read handshake
	packet, err := ReadPacket(reader)
	if err != nil {
		if cfg.Verbose {
			log.Println("Error reading handshake:", err)
		}
		clientConn.Close()
		return
	}
	handshake, err := DecodeServerBoundHandshake(packet)
	if err != nil {
		if cfg.Verbose {
			log.Printf("error while parsing handshake: %v\n", err)
		}
		clientConn.Close()
		return
	}

	// Check access
	userInfo := getUserInfoByHostname(handshake.Address)
	if userInfo == nil {
		if cfg.Verbose {
			log.Println("Remote addr: ", clientConn.RemoteAddr().String())
		}
		clientConn.Close()
		return
	}

	// Reset deadline
	clientConn.SetDeadline(time.Time{})

	if handshake.NextState == HandshakeStatus {
		handleStatusRequest(clientConn, handshake)
	} else if handshake.NextState == HandshakeLogin {
		handleLoginRequest(clientConn, handshake, userInfo)
	} else {
		if cfg.Verbose {
			log.Printf("Unknown handshake.NextState: %v\n", handshake.NextState)
		}
		clientConn.Close()
	}
}

func handleStatusRequest(clientConn net.Conn, handshake ServerBoundHandshake) {
	err := ProxyConnection(clientConn, cfg.MinecraftServer, handshake.ToPacket().Encode())
	if err != nil {
		var packet Packet
		packet.ID = 0
		status := StatusJSON{
			Version: StatusVersionJSON{
				Name:     "Some server",
				Protocol: int(handshake.ProtocolVersion),
			},
			Description: StatusDescriptionJSON{
				Text: "Offline",
			},
		}
		statusBytes, err := json.Marshal(status)
		if err != nil {
			clientConn.Close()
			return
		}
		packet.Data = McString(statusBytes).Encode()
		clientConn.Write(packet.Encode())
	}
}

func handleLoginRequest(clientConn net.Conn, handshake ServerBoundHandshake, userInfo *StorageRecord) {
	peekedData := handshake.ToPacket().Encode()

	packet, err := ReadPacket(bufio.NewReader(clientConn))
	if err != nil {
		if cfg.Verbose {
			log.Println("Error reading LoginStart:", err)
		}
		clientConn.Close()
		return
	}

	var passedUsername string
	if handshake.ProtocolVersion <= 758 { // 1.18.2 and older
		var login ServerLoginStartOLD
		login, err = DecodeServerBoundLoginStartOLD(packet)
		passedUsername = string(login.Nickname)
		login.Nickname = McString(userInfo.Nickname)
		peekedData = append(peekedData, login.ToPacket().Encode()...)

	} else if handshake.ProtocolVersion <= 760 { // 1.19 - 1.19.2
		// 759: 1.19
		// 760: 1.19.2
		var login ServerLoginStart759
		login, err = DecodeServerBoundLoginStart759(packet)
		passedUsername = string(login.Nickname)
		login.HasUUID = 1
		login.Nickname = McString(userInfo.Nickname)
		login.UUID = generateUUID(string(login.Nickname))
		peekedData = append(peekedData, login.ToPacket().Encode()...)
	} else if handshake.ProtocolVersion <= 763 { // 1.19.3 - 1.20.1
		// 761: 1.19.3
		// 762: 1.19.4
		// 763: 1.20 - 1.20.1
		var login ServerLoginStart761
		login, err = DecodeServerBoundLoginStart761(packet)
		passedUsername = string(login.Nickname)
		login.HasUUID = 1
		login.Nickname = McString(userInfo.Nickname)
		login.UUID = generateUUID(string(login.Nickname))
		peekedData = append(peekedData, login.ToPacket().Encode()...)
	} else { // 1.20.2 - last
		// 764: 1.20.2
		// 765: 1.20.3 - 1.20.4
		// 766: 1.20.5 - 1.20.6
		// 767: 1.21.1
		// 768: 1.21.2 - 1.21.3
		// 769: 1.21.4
		// 770: 1.21.5
		// 771: 1.21.6
		// 772: 1.21.7
		var login ServerLoginStart764
		login, err = DecodeServerBoundLoginStart764(packet)
		passedUsername = string(login.Nickname)
		login.Nickname = McString(userInfo.Nickname)
		login.UUID = generateUUID(string(login.Nickname))
		peekedData = append(peekedData, login.ToPacket().Encode()...)
	}

	if err != nil {
		log.Printf("error while parsing LoginStart: %v\n", err)
		clientConn.Close()
		return
	}

	// Get client IP without the port
	clientIP, _, _ := net.SplitHostPort(clientConn.RemoteAddr().String())
	// Authorize this IP for UDP traffic
	AuthorizeUDP(clientIP)
	// De-authorize the IP when the connection is closed
	defer DeauthorizeUDP(clientIP)

	addPlayer(userInfo.Nickname)
	updateOnlineMessage()
	log.Printf("User %s connected to %s from %s. Nickname %s -> %s\n", userInfo.TgName, cfg.BaseDomain, clientConn.RemoteAddr().String(), passedUsername, userInfo.Nickname)

	err = ProxyConnection(clientConn, cfg.MinecraftServer, peekedData)
	if err != nil {
		log.Print(err)
	}

	removePlayer(userInfo.Nickname)
	updateOnlineMessage()
	log.Printf("User %s disconnected. Nickname: %s\n", userInfo.TgName, userInfo.Nickname)
}

func handleResourcePackRequest(clientConn net.Conn, reader *bufio.Reader) {
	// Read the HTTP request
	request, err := http.ReadRequest(reader)
	if err != nil {
		log.Printf("Error reading HTTP request: %v\n", err)
		clientConn.Close()
		return
	}

	userInfo := getUserInfoByHostname(request.Host)
	if userInfo == nil {
		log.Printf("Reject HTTP request to: %s\n", request.URL.String())
		clientConn.Close()
		return
	}

	log.Printf("Transferring resource pack `%s` to %s", request.URL.String(), userInfo.TgName)

	// Create an HTTP client to proxy the request
	httpClient := &http.Client{
		Timeout: time.Second * 30,
	}

	// Create a new request to the target server
	proxyURL := "http://" + cfg.MinecraftServer + request.URL.Path
	proxyReq, err := http.NewRequest(request.Method, proxyURL, request.Body)
	if err != nil {
		log.Printf("Error creating proxy request: %v\n", err)
		clientConn.Close()
		return
	}

	// Copy headers
	proxyReq.Header = request.Header

	// Execute the request
	response, err := httpClient.Do(proxyReq)
	if err != nil {
		log.Printf("Error making proxy request: %v\n", err)
		clientConn.Close()
		return
	}
	defer response.Body.Close()

	// Send the response back to the client
	err = response.Write(clientConn)
	if err != nil {
		log.Printf("Error writing response: %v\n", err)
	}

	clientConn.Close()
}

///////////////////////////////////////////////////////////////////////////////

func generateUUID(username string) McUUID {
	username = "OfflinePlayer:" + username
	return NameUUIDFromBytes([]byte(username))
}

// https://github.com/AdoptOpenJDK/openjdk-jdk8u/blob/9a91972c76ddda5c1ce28b50ca38cbd8a30b7a72/jdk/src/share/classes/java/util/UUID.java#L153-L175
func NameUUIDFromBytes(name []byte) McUUID {
	var uuid McUUID

	// Вычисляем MD5 хэш от входных данных.
	hash := md5.Sum(name)

	// Устанавливаем версию UUID (версия 3).
	hash[6] &= 0x0f // Очистка верхних 4 бит.
	hash[6] |= 0x30 // Установка версии 3 (0011).

	// Устанавливаем вариант UUID (IETF).
	hash[8] &= 0x3f // Очистка верхних 2 бит.
	hash[8] |= 0x80 // Установка варианта (10).

	// Копируем хэш в UUID.
	copy(uuid[:], hash[:])

	return uuid
}

///////////////////////////////////////////////////////////////////////////////

var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 32*1024)
	},
}

func ProxyConnection(clientConn net.Conn, serverAddr string, peekedData []byte) (err error) {
	dialer := net.Dialer{
		Timeout: DialTimeout,
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(ProxyBind),
		},
	}

	serverConn, err := dialer.Dial("tcp", serverAddr)
	if err != nil {
		return err
	}

	_, err = serverConn.Write(peekedData)
	if err != nil {
		log.Printf("Error writing to server connection: %v\n", err)
		clientConn.Close()
		serverConn.Close()
		return err
	}

	go func() {
		buffer := bufferPool.Get().([]byte)
		defer bufferPool.Put(buffer)
		io.CopyBuffer(serverConn, clientConn, buffer)
		clientConn.Close()
	}()

	buffer := bufferPool.Get().([]byte)
	defer bufferPool.Put(buffer)
	io.CopyBuffer(clientConn, serverConn, buffer)
	serverConn.Close()

	return nil
}
