package main

import (
	"bufio"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	NetDeadline = time.Second * 2
	DialTimeout = time.Second * 2
	ProxyBind   = ""
)

func startMinecraftProxy() {
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
	defer listener.Close()

	fmt.Println("Proxy server listening on ", cfg.Listen)

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

	// Установка таймаута для предотвращения зависания при ожидании данных
	clientConn.SetDeadline(time.Now().Add(NetDeadline))

	// Попробуем прочитать первую строку как HTTP запрос
	firstLine, err := reader.Peek(7) // GET или HTTP
	if err == nil && (strings.HasPrefix(string(firstLine), "GET") || strings.HasPrefix(string(firstLine), "HTTP")) {
		handleResourcePackRequest(clientConn, reader)
		return
	}

	// Read handshake
	packet, err := ReadPacket(reader)
	if err != nil {
		log.Println("Error reading handshake:", err)
		clientConn.Close()
		return
	}
	handshake, err := DecodeServerBoundHandshake(packet)
	if err != nil {
		log.Printf("error while parsing handshake: %v\n", err)
		clientConn.Close()
		return
	}

	// Check access
	userInfo := getUserInfoByHostname(handshake.Address)
	if userInfo == nil {
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
		log.Printf("Unknown handshake.NextState: %v\n", err)
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
				Protocol: 769,
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
		log.Println("Error reading LoginStart:", err)
		clientConn.Close()
		return
	}

	var passedUsername string
	switch handshake.ProtocolVersion {

	// 1.18.2 and older
	default:
		var login ServerLoginStartOLD
		login, err = DecodeServerBoundLoginStartOLD(packet)
		passedUsername = string(login.Nickname)
		login.Nickname = McString(userInfo.Nickname)
		peekedData = append(peekedData, login.ToPacket().Encode()...)

	// 1.19 - 1.19.2
	case 759: // 1.19
		fallthrough
	case 760: // 1.19.2
		var login ServerLoginStart759
		login, err = DecodeServerBoundLoginStart759(packet)
		passedUsername = string(login.Nickname)
		login.HasUUID = 1
		login.Nickname = McString(userInfo.Nickname)
		login.UUID = generateUUID(string(login.Nickname))
		peekedData = append(peekedData, login.ToPacket().Encode()...)

	// 1.19.3 - 1.20.1
	case 761: // 1.19.3
		fallthrough
	case 762: // 1.19.4
		fallthrough
	case 763: // 1.20 - 1.20.1
		var login ServerLoginStart761
		login, err = DecodeServerBoundLoginStart761(packet)
		passedUsername = string(login.Nickname)
		login.HasUUID = 1
		login.Nickname = McString(userInfo.Nickname)
		login.UUID = generateUUID(string(login.Nickname))
		peekedData = append(peekedData, login.ToPacket().Encode()...)

	// 1.20.2 - last
	case 764: // 1.20.2
		fallthrough
	case 765: // 1.20.3 - 1.20.4
		fallthrough
	case 766: // 1.20.5 - 1.20.6
		fallthrough
	case 767: // 1.21.1
		fallthrough
	case 768: // 1.21.2
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

	log.Printf("User %s connected to %s. Username %s -> %s\n", userInfo.TgName, cfg.BaseDomain, passedUsername, userInfo.Nickname)

	ProxyConnection(clientConn, cfg.MinecraftServer, peekedData)
}

func handleResourcePackRequest(clientConn net.Conn, reader *bufio.Reader) {
	// Читаем HTTP запрос
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

	// Создаем HTTP клиент для проксирования запроса
	httpClient := &http.Client{
		Timeout: time.Second * 30,
	}

	// Создаем новый запрос к целевому серверу
	proxyURL := "http://" + cfg.MinecraftServer + request.URL.Path
	proxyReq, err := http.NewRequest(request.Method, proxyURL, request.Body)
	if err != nil {
		log.Printf("Error creating proxy request: %v\n", err)
		clientConn.Close()
		return
	}

	// Копируем заголовки
	proxyReq.Header = request.Header

	// Выполняем запрос
	response, err := httpClient.Do(proxyReq)
	if err != nil {
		log.Printf("Error making proxy request: %v\n", err)
		clientConn.Close()
		return
	}
	defer response.Body.Close()

	// Отправляем ответ клиенту
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

	serverConn.Write(peekedData)

	go func() {
		pipe(serverConn, clientConn)
		clientConn.Close()
	}()

	go func() {
		pipe(clientConn, serverConn)
		serverConn.Close()
	}()

	return nil
}

func pipe(c1, c2 net.Conn) {
	buffer := make([]byte, 0xffff)
	for {
		n, err := c1.Read(buffer)
		if err != nil {
			return
		}
		_, err = c2.Write(buffer[:n])
		if err != nil {
			return
		}
	}
}
