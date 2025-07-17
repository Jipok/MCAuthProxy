package main

import (
	"log"
	"net"
	"sync"
	"time"
)

var (
	// udpSessions maps a client's address (ip:port) to its corresponding connection to the game server
	udpSessions      = make(map[string]*net.UDPConn)
	udpSessionsMutex = &sync.RWMutex{}

	// authedClients holds the IP addresses of clients who have successfully authenticated via TCP.
	// We only allow UDP traffic from these IPs.
	authedClients      = make(map[string]bool)
	authedClientsMutex = &sync.RWMutex{}
)

// AuthorizeUDP adds a client's IP to the whitelist for UDP traffic.
func AuthorizeUDP(clientIP string) {
	authedClientsMutex.Lock()
	defer authedClientsMutex.Unlock()
	authedClients[clientIP] = true
}

// DeauthorizeUDP removes a client's IP from the UDP whitelist.
func DeauthorizeUDP(clientIP string) {
	authedClientsMutex.Lock()
	defer authedClientsMutex.Unlock()
	delete(authedClients, clientIP)
}

// startUdpProxy listens for incoming UDP packets and forwards them.
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

	// Use a buffer to read incoming packets.
	buffer := make([]byte, 4096)

	for {
		n, clientAddr, err := udpConn.ReadFromUDP(buffer)
		if err != nil {
			// Ignore read errors and continue.
			continue
		}

		go proxyUdpPacket(udpConn, clientAddr, buffer[:n], serverAddr)
	}
}

// proxyUdpPacket handles a single UDP packet from a client.
func proxyUdpPacket(proxyListener *net.UDPConn, clientAddr *net.UDPAddr, data []byte, serverAddr string) {
	clientIP, _, _ := net.SplitHostPort(clientAddr.String())

	// Check if the client's IP is authorized.
	authedClientsMutex.RLock()
	isAuthed := authedClients[clientIP]
	authedClientsMutex.RUnlock()

	if !isAuthed {
		// Drop packet if the source IP is not from an active TCP session.
		return
	}

	// Check if we already have a UDP session for this client address.
	udpSessionsMutex.RLock()
	serverConn, found := udpSessions[clientAddr.String()]
	udpSessionsMutex.RUnlock()

	// If no session exists, create a new one.
	if !found {
		// Resolve the destination Minecraft server's address.
		serverUdpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
		if err != nil {
			log.Printf("UDP: Error resolving Minecraft server address: %v", err)
			return
		}

		// Dial the server to create a new UDP "connection".
		newServerConn, err := net.DialUDP("udp", nil, serverUdpAddr)
		if err != nil {
			log.Printf("UDP: Error dialing server: %v", err)
			return
		}

		serverConn = newServerConn

		// Store the new session in our map.
		udpSessionsMutex.Lock()
		udpSessions[clientAddr.String()] = serverConn
		udpSessionsMutex.Unlock()

		// Start a goroutine to listen for replies from the server
		// and forward them back to the client.
		// log.Printf("UDP: Created new session for %s", clientAddr.String())
		go func() {
			// This defer ensures that the session is cleaned up when this goroutine exits.
			defer func() {
				udpSessionsMutex.Lock()
				delete(udpSessions, clientAddr.String())
				udpSessionsMutex.Unlock()
				serverConn.Close()
				// log.Printf("UDP: Closed session for %s", clientAddr.String())
			}()

			replyBuffer := make([]byte, 4096)
			for {
				// Set a deadline on reads. If no data comes from the server for 30 seconds,
				// the connection is considered stale, and the goroutine will exit.
				serverConn.SetReadDeadline(time.Now().Add(30 * time.Second))

				n, _, err := serverConn.ReadFromUDP(replyBuffer)
				if err != nil {
					// Likely a timeout, which is our signal to close the session.
					return
				}

				// Forward the server's reply back to the client using the main listener.
				_, err = proxyListener.WriteToUDP(replyBuffer[:n], clientAddr)
				if err != nil {
					return
				}
			}
		}()
	}

	// Forward the client's packet to the game server.
	_, err := serverConn.Write(data)
	if err != nil {
		log.Printf("UDP: Error writing to server: %v", err)
	}

	// Each time the client sends a packet, we extend the life of their UDP session.
	serverConn.SetReadDeadline(time.Now().Add(30 * time.Second))
}
