package main

import (
	"bufio"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// Record represents a single storage entry
type StorageRecord struct {
	Token    string
	Nickname string
	ID       int64
	TgName   string
}

// Storage implements thread-safe storage for records
type Storage struct {
	filename string
	mu       sync.RWMutex
}

// NewStorage creates a new storage instance
func NewStorage(filename string) *Storage {
	return &Storage{
		filename: filename,
	}
}

var (
	ErrNicknameExists   = errors.New("nickname already exists")
	ErrNicknameNotFound = errors.New("nickname not found")
	ErrAccessDenied     = errors.New("attempt to delete a nickname that does not belong to")
)

// generateUniqueToken creates a random 15-character token and ensures it's unique
func (s *Storage) generateUniqueToken() (string, error) {
	records, err := s.readRecords()
	if err != nil {
		return "", err
	}

	// Create a map of existing tokens for faster lookup
	existingTokens := make(map[string]struct{})
	for _, r := range records {
		existingTokens[r.Token] = struct{}{}
	}

	// Try to generate unique token with maximum attempts
	maxAttempts := 100
	for i := 0; i < maxAttempts; i++ {
		token := generateToken()
		if _, exists := existingTokens[token]; !exists {
			return token, nil
		}
	}

	return "", errors.New("failed to generate unique token after maximum attempts")
}

// generateToken creates a random 15-character token
func generateToken() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	token := make([]byte, 20)
	rand.Read(token)
	for i := range token {
		token[i] = charset[int(token[i])%len(charset)]
	}
	return string(token)
}

// AddRecord adds a new record to storage
func (s *Storage) AddRecord(nickname, tgname string, id int64) (*StorageRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if nickname already exists
	records, err := s.readRecords()
	if err != nil {
		return nil, err
	}

	for _, r := range records {
		if strings.EqualFold(r.Nickname, nickname) {
			return nil, ErrNicknameExists
		}
	}

	token, err := s.generateUniqueToken()
	if err != nil {
		return nil, err
	}

	// Create new record
	record := &StorageRecord{
		Token:    token,
		Nickname: nickname,
		ID:       id,
		TgName:   tgname,
	}

	// Append to file
	f, err := os.OpenFile(s.filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "%s\t%s\t%d\t%s\n", record.Token, record.Nickname, record.ID, record.TgName)
	if err != nil {
		return nil, err
	}

	return record, nil
}

// FindByToken searches for a record by token
func (s *Storage) FindByToken(token string) (*StorageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	records, err := s.readRecords()
	if err != nil {
		return nil, err
	}

	for _, r := range records {
		if r.Token == token {
			return &r, nil
		}
	}

	return nil, errors.New("record not found")
}

// FindByTgID returns all records with matching telegram ID
func (s *Storage) FindByTgID(id int64) ([]StorageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	records, err := s.readRecords()
	if err != nil {
		return nil, err
	}

	var result []StorageRecord
	for _, r := range records {
		if r.ID == id {
			result = append(result, r)
		}
	}

	return result, nil
}

// DeleteByUsername removes a record by nickname
func (s *Storage) DeleteByNickname(nickname string, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.readRecords()
	if err != nil {
		return err
	}

	var newRecords []StorageRecord
	found := false
	for _, r := range records {
		if r.Nickname != nickname {
			newRecords = append(newRecords, r)
		} else {
			if r.ID != id {
				return ErrAccessDenied
			}
			found = true
		}
	}

	if !found {
		return ErrNicknameNotFound
	}

	// Rewrite the entire file
	return s.writeRecords(newRecords)
}

// readRecords reads all records from file
func (s *Storage) readRecords() ([]StorageRecord, error) {
	f, err := os.OpenFile(s.filename, os.O_CREATE|os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []StorageRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), "\t", 4)
		if len(fields) != 4 {
			continue
		}

		var id int64
		fmt.Sscanf(fields[2], "%d", &id)

		record := StorageRecord{
			Token:    fields[0],
			Nickname: fields[1],
			ID:       id,
			TgName:   fields[3],
		}
		records = append(records, record)
	}

	return records, scanner.Err()
}

// writeRecords writes all records to file
func (s *Storage) writeRecords(records []StorageRecord) error {
	f, err := os.Create(s.filename)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, r := range records {
		_, err := fmt.Fprintf(f, "%s\t%s\t%d\t%s\n", r.Token, r.Nickname, r.ID, r.TgName)
		if err != nil {
			return err
		}
	}

	return nil
}
