package bw

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/netbrain/bwmenu/internal/debugflag"
	"github.com/netbrain/bwmenu/internal/keychain"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// Manager is the Bitwarden manager interface used by the UI.
type Manager interface {
	IsInstalled() bool
	IsLoggedIn() (bool, error)
	GetItems() ([]map[string]interface{}, error)
	GetPassword(id string) (string, error)
	GetTotp(id string) (string, error)
	Unlock(password string) (string, error)
}

// Process (bw CLI) implementation

type ProcessManager struct{}

func NewProcessManager() Manager { return &ProcessManager{} }

func (b *ProcessManager) IsInstalled() bool {
	_, err := exec.LookPath("bw")
	return err == nil
}

func (b *ProcessManager) IsLoggedIn() (bool, error) {
	if debugflag.Enabled {
		log.Println("Checking for session key in keychain...")
	}
	sessionKey, err := keychain.GetSessionKey()
	if err == nil && sessionKey != "" {
		if debugflag.Enabled {
			log.Println("Found session key.")
		}
		os.Setenv("BW_SESSION", sessionKey)
		return true, nil
	}

	if debugflag.Enabled {
		log.Printf("Could not get session key: %v", err)
		log.Println("Checking for BW_SESSION environment variable...")
	}
	if os.Getenv("BW_SESSION") != "" {
		if debugflag.Enabled {
			log.Println("Found BW_SESSION environment variable.")
		}
		return true, nil
	}

	if debugflag.Enabled {
		log.Println("BW_SESSION environment variable not set. Checking bw status...")
	}
	cmd := exec.Command("bw", "status")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	var status struct{ Status string `json:"status"` }
	if err := json.Unmarshal(out, &status); err != nil {
		return false, err
	}
	if debugflag.Enabled {
		log.Printf("bw status: %s", status.Status)
	}
	if status.Status != "unlocked" {
		if debugflag.Enabled { log.Println("Session is locked.") }
		keychain.DeleteSessionKey()
	}
	return status.Status == "unlocked", nil
}

func (b *ProcessManager) GetItems() ([]map[string]interface{}, error) {
	cmd := exec.Command("bw", "list", "items")
	out, err := cmd.Output()
	if err != nil { return nil, err }
	if len(out) == 0 || !json.Valid(out) { return []map[string]interface{}{}, nil }
	var items []map[string]interface{}
	if err := json.Unmarshal(out, &items); err != nil { return nil, err }
	return items, nil
}

func (b *ProcessManager) GetPassword(id string) (string, error) {
	out, err := exec.Command("bw", "get", "password", id).Output()
	if err != nil { return "", err }
	return string(out), nil
}

func (b *ProcessManager) GetTotp(id string) (string, error) {
	out, err := exec.Command("bw", "get", "totp", id).Output()
	if err != nil { return "", err }
	return string(out), nil
}

func (b *ProcessManager) Unlock(password string) (string, error) {
	out, err := exec.Command("bw", "unlock", password, "--raw").Output()
	if err != nil { return "", err }
	sessionKey := string(out)
	if err := keychain.SetSessionKey(sessionKey); err != nil { return "", err }
	return sessionKey, nil
}

// API implementation

type APIManager struct{ apiUrl string }

func NewAPIManager(apiUrl string) Manager { return &APIManager{apiUrl: apiUrl} }

func (b *APIManager) IsInstalled() bool { return true }

func (b *APIManager) IsLoggedIn() (bool, error) {
	req, err := http.NewRequest("GET", b.apiUrl+"/status", nil)
	if err != nil { return false, err }
	resp, err := (&http.Client{}).Do(req)
	if err != nil { return false, err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { return false, fmt.Errorf("status check failed: %s", resp.Status) }
	var statusResponse struct {
		Success bool `json:"success"`
		Data    struct{ Template struct{ Status string `json:"status"` } }
	}
	if err := json.NewDecoder(resp.Body).Decode(&statusResponse); err != nil { return false, err }
	return statusResponse.Success && statusResponse.Data.Template.Status == "unlocked", nil
}

func (b *APIManager) GetItems() ([]map[string]interface{}, error) {
	req, err := http.NewRequest("GET", b.apiUrl+"/list/object/items", nil)
	if err != nil { return nil, err }
	resp, err := (&http.Client{}).Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil { return nil, fmt.Errorf("failed to read response body: %w", err) }
	resp.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	if debugflag.Enabled { log.Printf("GetItems response status: %s", resp.Status) }
	if resp.StatusCode != http.StatusOK { return nil, fmt.Errorf("get items failed: %s", resp.Status) }
	var response struct {
		Success bool `json:"success"`
		Data    struct{ Object string `json:"object"`; Data []map[string]interface{} `json:"data"` }
	}
	if err := json.Unmarshal(bodyBytes, &response); err != nil { return nil, err }
	if !response.Success { return nil, fmt.Errorf("get items failed: %s", response.Data.Object) }
	return response.Data.Data, nil
}

func (b *APIManager) getItem(id string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", b.apiUrl+"/object/item/"+id, nil)
	if err != nil { return nil, err }
	resp, err := (&http.Client{}).Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil { return nil, fmt.Errorf("failed to read response body: %w", err) }
	resp.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	if debugflag.Enabled { log.Printf("getItem response status: %s", resp.Status) }
	if resp.StatusCode != http.StatusOK {
		if debugflag.Enabled { log.Printf("getItem non-OK status code: %s, body: %s", resp.Status, string(bodyBytes)) }
		return nil, fmt.Errorf("get item failed: %s", resp.Status)
	}
	var item map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &item); err != nil {
		if debugflag.Enabled { log.Printf("getItem JSON unmarshal error: %v, body: %s", err, string(bodyBytes)) }
		return nil, err
	}
	if debugflag.Enabled { log.Printf("getItem parsed item: %+v", item) }
	return item, nil
}

func (b *APIManager) GetPassword(id string) (string, error) {
	if debugflag.Enabled { log.Printf("Calling getItem for ID: %s", id) }
	item, err := b.getItem(id)
	if err != nil { if debugflag.Enabled { log.Printf("Error getting item %s: %v", id, err) }; return "", err }
	if data, ok := item["data"].(map[string]interface{}); ok {
		if login, ok := data["login"].(map[string]interface{}); ok {
			if password, ok := login["password"].(string); ok { return password, nil }
		}
	}
	return "", fmt.Errorf("password not found")
}

func (b *APIManager) GetTotp(id string) (string, error) {
	if debugflag.Enabled { log.Printf("Calling getItem for ID: %s", id) }
	item, err := b.getItem(id)
	if err != nil { if debugflag.Enabled { log.Printf("Error getting item %s: %v", id, err) }; return "", err }
	if data, ok := item["data"].(map[string]interface{}); ok {
		if login, ok := data["login"].(map[string]interface{}); ok {
			if totpURL, ok := login["totp"].(string); ok {
				otpKey, err := otp.NewKeyFromURL(totpURL)
				if err != nil { return "", fmt.Errorf("failed to parse OTP URL: %w", err) }
				otpCode, err := totp.GenerateCode(otpKey.Secret(), time.Now())
				if err != nil { return "", fmt.Errorf("failed to generate OTP code: %w", err) }
				return otpCode, nil
			}
		}
	}
	return "", fmt.Errorf("totp not found")
}

func (b *APIManager) Unlock(password string) (string, error) {
	body, err := json.Marshal(map[string]string{"password": password})
	if err != nil { return "", err }
	req, err := http.NewRequest("POST", b.apiUrl+"/unlock", bytes.NewBuffer(body))
	if err != nil { return "", err }
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{}).Do(req)
	if err != nil { return "", err }
	defer resp.Body.Close()
	var unlockResponse struct {
		Success bool `json:"success"`
		Message string `json:"message"`
		Data    struct{ Raw string `json:"raw"` }
	}
	if err := json.NewDecoder(resp.Body).Decode(&unlockResponse); err != nil { return "", err }
	if !unlockResponse.Success { return "", fmt.Errorf("unlock failed: %s", unlockResponse.Message) }
	sessionKey := unlockResponse.Data.Raw
	if err := keychain.SetSessionKey(sessionKey); err != nil { return "", err }
	return sessionKey, nil
}

