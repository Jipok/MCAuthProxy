package main

import (
	"log"
	"net"
	"runtime"
	"sync"
)

// udpPacket represents a single UDP datagram to be processed by a worker.
// It bundles the client's address and the data, and crucially, the original
// buffer from the pool so the worker can return it.
type udpPacket struct {
	clientAddr *net.UDPAddr
	data       []byte // A slice pointing to the data within the buffer
	buffer     []byte // The original buffer that must be returned to the pool
}

var (
	// udpSessions maps a client's address (ip:port) to its corresponding connection to the game server
	udpSessions      = make(map[string]*net.UDPConn)
	udpSessionsMutex = &sync.RWMutex{}

	// authedClients holds the count of active TCP sessions for each client IP.
	// We only allow UDP traffic from IPs with a count > 0.
	authedClients      = make(map[string]int)
	authedClientsMutex = &sync.RWMutex{}

	// packetChan is a buffered channel that acts as a queue between the main
	// UDP listener and the worker pool.
	packetChan chan udpPacket
)

const (
	udpBufferSize   = 4096
	packetQueueSize = 1024
)

// AuthorizeUDP increments the active session count for a client's IP
func AuthorizeUDP(clientIP string) {
	authedClientsMutex.Lock()
	defer authedClientsMutex.Unlock()
	authedClients[clientIP]++
}

// DeauthorizeUDP decrements the reference count for a client's IP.
// If the count reaches zero, it removes the IP from the whitelist and closes all associated UDP sessions.
func DeauthorizeUDP(clientIP string) {
	authedClientsMutex.Lock()
	authedClients[clientIP]--
	count := authedClients[clientIP]
	if count <= 0 {
		delete(authedClients, clientIP)
	}
	authedClientsMutex.Unlock()

	// If the count > 0, it means other players are still connected from this IP,
	// so we do not close any UDP sessions.
	if count > 0 {
		return
	}

	// If we are here, it means no more authorized clients are connected from this IP.
	// We can safely close all UDP sessions associated with it.
	var sessionsToClose []*net.UDPConn

	udpSessionsMutex.RLock()
	for addr, conn := range udpSessions {
		ip, _, err := net.SplitHostPort(addr)
		if err == nil && ip == clientIP {
			sessionsToClose = append(sessionsToClose, conn)
		}
	}
	udpSessionsMutex.RUnlock()

	for _, conn := range sessionsToClose {
		conn.Close()
	}
}

// udpBufferPool stores and reuses buffers to reduce garbage collection overhead
var udpBufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, udpBufferSize)
	},
}

// startUdpProxy listens for incoming UDP packets and forwards them
func startUdpProxy(listenAddr, serverAddr string) {
	udpAddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		log.Fatalf("UDP: Error resolving address: %v", err)
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatalf("UDP: Error starting listener: %v", err)
	}
	defer udpConn.Close()
	log.Println("Proxy server listening on (UDP)", listenAddr)

	// The channel buffer acts as a queue to absorb temporary spikes in traffic,
	// preventing the listener from blocking if workers are busy.
	// The size can be tuned based on expected load.
	packetChan = make(chan udpPacket, packetQueueSize)

	// Using a number of workers equal to the number of CPU cores is a
	// good starting point to balance concurrency and resource usage.
	numWorkers := runtime.NumCPU()
	for i := 0; i < numWorkers; i++ {
		go udpWorker(udpConn, serverAddr)
	}

	for {
		// Get a buffer from the pool
		buffer := udpBufferPool.Get().([]byte)

		n, clientAddr, err := udpConn.ReadFromUDP(buffer)
		if err != nil {
			// If reading fails, we MUST return the buffer to the pool
			// before continuing the loop, otherwise it's lost.
			udpBufferPool.Put(buffer)
			log.Printf("UDP: Error reading from UDP: %v", err)
			continue
		}

		// Send the work to the channel.
		// This is a non-blocking operation if the channel buffer has space.
		packetChan <- udpPacket{
			clientAddr: clientAddr,
			data:       buffer[:n],
			buffer:     buffer, // Pass the full buffer for a proper return to the pool
		}
	}
}

// udpWorker is a long-lived goroutine that receives packets from packetChan
// and processes them.
func udpWorker(proxyListener *net.UDPConn, serverAddr string) {
	for packet := range packetChan {
		// Process the packet using the original logic.
		proxyUdpPacket(proxyListener, packet.clientAddr, packet.data, serverAddr)

		// After the packet is processed, return its buffer to the pool.
		// This is the worker's responsibility now.
		udpBufferPool.Put(packet.buffer)
	}
}

// proxyUdpPacket handles a single UDP packet from a client.
func proxyUdpPacket(proxyListener *net.UDPConn, clientAddr *net.UDPAddr, data []byte, serverAddr string) {
	clientIP, _, _ := net.SplitHostPort(clientAddr.String())

	// Check if the client's IP is authorized.
	authedClientsMutex.RLock()
	count, isAuthed := authedClients[clientIP]
	authedClientsMutex.RUnlock()

	if !isAuthed || count <= 0 {
		// Drop packet if the source IP has no active TCP sessions
		return
	}

	// Check if we already have a UDP session for this client address.
	udpSessionsMutex.RLock()
	serverConn, found := udpSessions[clientAddr.String()]
	udpSessionsMutex.RUnlock()

	// If no session exists, create a new one.
	if !found {
		// Acquire a full lock to prevent race conditions on creation.
		udpSessionsMutex.Lock()
		// Check again, in case another goroutine created the session while we were waiting for the lock.
		serverConn, found = udpSessions[clientAddr.String()]
		if !found {
			// Resolve the destination Minecraft server's address.
			serverUdpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
			if err != nil {
				log.Printf("UDP: Error resolving Minecraft server address: %v", err)
				udpSessionsMutex.Unlock()
				return
			}

			// Dial the server to create a new UDP "connection".
			newServerConn, err := net.DialUDP("udp", nil, serverUdpAddr)
			if err != nil {
				log.Printf("UDP: Error dialing server: %v", err)
				udpSessionsMutex.Unlock()
				return
			}

			serverConn = newServerConn
			udpSessions[clientAddr.String()] = serverConn

			// Start a goroutine to listen for replies from the server
			go listenForServerReplies(proxyListener, clientAddr, serverConn)
		}
		udpSessionsMutex.Unlock()
	}

	// Forward the client's packet to the game server
	_, err := serverConn.Write(data)
	if err != nil {
		log.Printf("UDP: Error writing to server: %v", err)
	}
}

func listenForServerReplies(proxyListener *net.UDPConn, clientAddr *net.UDPAddr, serverConn *net.UDPConn) {
	// This defer ensures that the session is cleaned up when this goroutine exits.
	// It will be triggered when serverConn.Close() is called from another goroutine (DeauthorizeUDP).
	defer func() {
		udpSessionsMutex.Lock()
		delete(udpSessions, clientAddr.String())
		udpSessionsMutex.Unlock()
		serverConn.Close()
		// log.Printf("UDP: Closed session for %s", clientAddr.String())
	}()

	replyBuffer := make([]byte, udpBufferSize)
	for {
		n, _, err := serverConn.ReadFromUDP(replyBuffer)
		if err != nil {
			// This error is expected when conn.Close() is called from DeauthorizeUDP.
			// The goroutine will now exit, and the defer will clean up the session.
			return
		}

		// Forward the server's reply back to the client using the main listener.
		_, err = proxyListener.WriteToUDP(replyBuffer[:n], clientAddr)
		if err != nil {
			// If we can't write back to the client, the session is likely broken.
			return
		}
	}
}
