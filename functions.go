package valheimdiscord

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"github.com/bsdlp/discord-interactions-go/interactions"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
)

const (
	pubKeyEnv           = "DISCORD_PUBKEY"
	webhookUrlEnv       = "DISCORD_WEBHOOK_URL"
	gcpProjEnv          = "GCP_PROJECT"
	gcpZoneEnv          = "GCP_ZONE"
	gcpInstanceNameEnv  = "GCP_INSTANCE_NAME"
	statusServerPortEnv = "STATUS_SERVER_PORT"
	failedMessage       = "Something broke and I couldn't get to the server :("
)

var (
	pubKeyHex        = os.Getenv(pubKeyEnv)
	webhookUrl       = os.Getenv(webhookUrlEnv)
	gcpProj          = os.Getenv(gcpProjEnv)
	gcpZone          = os.Getenv(gcpZoneEnv)
	gcpInstanceName  = os.Getenv(gcpInstanceNameEnv)
	statusServerPort = os.Getenv(statusServerPortEnv)
)

func DiscordWebhook(w http.ResponseWriter, r *http.Request) {
	pubKey, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		http.Error(w, "invalid discord public key", http.StatusUnauthorized)
		return
	}

	verified := interactions.Verify(r, ed25519.PublicKey(pubKey))
	if !verified {
		http.Error(w, "signature mismatch", http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()
	var data interactions.Data
	if err = json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "could not read interaction", http.StatusBadRequest)
		return
	}

	switch data.Type {
	case interactions.Ping:
		handlePing(w)
	case interactions.ApplicationCommand:
		handleAppCommand(w, data)
	default:
		http.Error(w, "unknown command", http.StatusBadRequest)
	}
}

func handlePing(w http.ResponseWriter) {
	if _, err := w.Write([]byte(`{"type":1}`)); err != nil {
		log.Printf("failed to respond to ping: %v", err)
	}
}

func handleAppCommand(w http.ResponseWriter, data interactions.Data) {
	message, alsoSendWebhook := executeCommand(data)
	response := interactions.InteractionResponse{
		Type: interactions.ChannelMessageWithSource,
		Data: &interactions.InteractionApplicationCommandCallbackData{
			Content: message,
			Flags:   interactions.Ephemeral,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("failed to write encode response: %v", err)
		return
	}

	if alsoSendWebhook {
		err := sendWebhook(message)
		if err != nil {
			log.Printf("failed to send webhook: %v", err)
		}
	}
}

func executeCommand(data interactions.Data) (string, bool) {
	if data.Data.Name != "valheim" {
		return "I don't recognize your command :(", false
	}

	ctx := context.Background()
	c, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		log.Printf("failed to connect to gcp: %v", err)
		return failedMessage, false
	}

	defer c.Close()
	instance, err := getValheimInstance(ctx, c)
	if instance == nil || err != nil {
		log.Printf("failed to find the instance: %v", err)
		return failedMessage, false
	}

	status := instance.GetStatus()
	switch data.Data.Options[0].Name {
	case "status":
		statusMessage := formatInstanceStatusMessage(status)
		if status == computepb.Instance_RUNNING {
			uptimeMessage, err := formatInstanceUptimeMessage(instance.GetLastStartTimestamp())
			if err != nil {
				log.Printf("couldn't get server uptime: %v", err)
			} else {
				statusMessage = fmt.Sprintf("%v %v", statusMessage, uptimeMessage)
			}

			playerCount, err := getPlayerCount(instance)
			if err != nil {
				log.Printf("couldn't get player count: %v", err)
			} else {
				statusMessage = fmt.Sprintf("%v %v", statusMessage, formatPlayerCountMessage(playerCount))
			}
		}

		return statusMessage, false

	case "start":
		if status != computepb.Instance_TERMINATED {
			log.Printf("refusing to start, server is not in terminated state")
			return "The server is already started.", false
		}

		err := startInstance(ctx, c)
		if err != nil {
			log.Printf("failed to start server: %v", err)
			return "Couldn't start the server :(", true
		}

		return "The server has been started! It should be up soon!", true

	case "stop":
		if status != computepb.Instance_RUNNING {
			log.Printf("refusing stop, server status is not in running state")
			return "The server is already shut down.", false
		}

		playerCount, err := getPlayerCount(instance)
		if err != nil {
			log.Printf("couldn't get player count: %v", err)
		} else if playerCount > 0 {
			return "There are %v people online, refusing to shut down.", false
		}

		err = stopInstance(ctx, c)
		if err != nil {
			log.Printf("failed to stop server: %v", err)
			return "Couldn't stop the server :(", true
		} else {

			return "The server is shutting down.", true
		}

	default:
		return "I didn't recognize your command :(", false
	}
}

func getValheimInstance(ctx context.Context, client *compute.InstancesClient) (*computepb.Instance, error) {
	return client.Get(ctx, &computepb.GetInstanceRequest{
		Project:  gcpProj,
		Zone:     gcpZone,
		Instance: gcpInstanceName,
	})
}

func formatInstanceStatusMessage(status computepb.Instance_Status) string {
	switch status {
	case computepb.Instance_RUNNING:
		return "The server is running!"
	case computepb.Instance_STOPPING:
		return "The server is shutting down..."
	case computepb.Instance_TERMINATED:
		return "The server is shut down."
	default:
		// just being lazy, the 3 cases above cover the states i care about
		return "The server is in a mysterious state."
	}
}

func formatInstanceUptimeMessage(lastStartTime string) (string, error) {
	startTime, err := time.Parse(time.RFC3339, lastStartTime)
	if err != nil {
		log.Printf("failed to parse time: %v", err)
		return "", err
	}

	return fmt.Sprintf("It has been up for %s.", time.Since(startTime).Truncate(1*time.Minute)), nil
}

func getInstanceExternalIP(instance *computepb.Instance) (string, error) {
	for _, i := range instance.GetNetworkInterfaces() {
		for _, ac := range i.GetAccessConfigs() {
			if ac.NatIP != nil {
				return *ac.NatIP, nil
			}
		}
	}

	return "", fmt.Errorf("could not find an external ip")
}

func startInstance(ctx context.Context, client *compute.InstancesClient) error {
	_, err := client.Start(ctx, &computepb.StartInstanceRequest{
		Project:  gcpProj,
		Zone:     gcpZone,
		Instance: gcpInstanceName,
	})
	return err
}

func stopInstance(ctx context.Context, client *compute.InstancesClient) error {
	_, err := client.Stop(ctx, &computepb.StopInstanceRequest{
		Project:  gcpProj,
		Zone:     gcpZone,
		Instance: gcpInstanceName,
	})
	return err
}

func sendWebhook(message string) error {
	msgJson := fmt.Sprintf(`{"username": "valheimbot", "content": "%v"}`, message)
	_, err := http.Post(webhookUrl, "application/json", strings.NewReader(msgJson))
	return err
}

type ValheimServerStatus struct {
	ServerName  string `json:"server_name,omitempty"`
	PlayerCount int    `json:"player_count,omitempty"`
	Error       string `json:"error,omitempty"`
}

func getValheimServerStatus(statusJsonUrl string) (ValheimServerStatus, error) {
	client := http.Client{
		Timeout: 1 * time.Second,
	}

	resp, err := client.Get(statusJsonUrl)
	if err != nil {
		return ValheimServerStatus{}, err
	}

	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ValheimServerStatus{}, err
	}

	status := ValheimServerStatus{}
	err = json.Unmarshal(respBytes, &status)
	return status, err
}

func getPlayerCount(instance *computepb.Instance) (int, error) {
	ip, err := getInstanceExternalIP(instance)
	if err != nil {
		return 0, err
	}

	vhStatus, err := getValheimServerStatus(fmt.Sprintf("http://%v:%v/status.json", ip, statusServerPort))
	if err != nil {
		return 0, err
	}

	return vhStatus.PlayerCount, nil
}

func formatPlayerCountMessage(playerCount int) string {
	message := ""
	if playerCount == 0 {
		message = "There's no one playing."
	} else if playerCount == 1 {
		message = "There's 1 person online. They're probably lonely, go join them!"
	} else {
		message = fmt.Sprintf("There's %v people online.", playerCount)
	}

	return message
}
