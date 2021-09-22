package valheimdiscord

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	pubKeyEnv          = "DISCORD_PUBKEY"
	webhookUrlEnv      = "DISCORD_WEBHOOK_URL"
	gcpProjEnv         = "GCP_PROJECT"
	gcpZoneEnv         = "GCP_ZONE"
	gcpInstanceNameEnv = "GCP_INSTANCE_NAME"
	failedMessage      = "Something broke and I couldn't get to the server :("
)

var (
	pubKeyHex       = os.Getenv(pubKeyEnv)
	webhookUrl      = os.Getenv(webhookUrlEnv)
	gcpProj         = os.Getenv(gcpProjEnv)
	gcpZone         = os.Getenv(gcpZoneEnv)
	gcpInstanceName = os.Getenv(gcpInstanceNameEnv)
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
	message := executeCommand(data)
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
}

func executeCommand(data interactions.Data) string {
	if data.Data.Name != "valheim" {
		return "I don't recognize your command :("
	}

	ctx := context.Background()
	c, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		log.Printf("failed to connect to gcp: %v", err)
		return failedMessage
	}

	defer c.Close()
	instance, err := getValheimComputeInstance(ctx, c)
	if instance == nil || err != nil {
		log.Printf("failed to find the instance: %v", err)
		return failedMessage
	}

	statusMessage := ""
	switch data.Data.Options[0].Name {
	case "status":
		// we only send the status to the user who requested it
		return getStatusMessage(instance)
	case "start":
		err := startValheimServer(ctx, c)
		if err != nil {
			log.Printf("failed to start server: %v", err)
			statusMessage = "Couldn't start the server :("
		} else {
			statusMessage = "The server has been started! It should be up soon!"
		}
	case "stop":
		err := stopValheimServer(ctx, c)
		if err != nil {
			log.Printf("failed to stop server: %v", err)
			statusMessage = "Couldn't stop the server :("
		} else {

			statusMessage = "The server is shutting down"
		}
	default:
		return "I didn't recognize your command :("
	}

	err = sendWebhook(statusMessage)
	if err != nil {
		log.Printf("failed to send webhook: %v", err)
	}

	return statusMessage
}

func getValheimComputeInstance(ctx context.Context, client *compute.InstancesClient) (*computepb.Instance, error) {
	return client.Get(ctx, &computepb.GetInstanceRequest{
		Project:  gcpProj,
		Zone:     gcpZone,
		Instance: gcpInstanceName,
	})
}

func getStatusMessage(instance *computepb.Instance) string {
	status := instance.GetStatus()
	switch status {
	case computepb.Instance_RUNNING:
		startTime, err := time.Parse(time.RFC3339, instance.GetLastStartTimestamp())
		if err != nil {
			log.Printf("failed to parse time: %v", err)
			return "The server is running!"
		}

		return fmt.Sprintf("The server is running. It has been up for %s", time.Since(startTime).Truncate(1*time.Minute))
	case computepb.Instance_STOPPING:
		return "The server is shutting down..."
	case computepb.Instance_TERMINATED:
		return "The server is shut down."
	default:
		// just being lazy, the 3 cases above cover the states i care about
		return "The server is in a mysterious state"
	}
}

func startValheimServer(ctx context.Context, client *compute.InstancesClient) error {
	_, err := client.Start(ctx, &computepb.StartInstanceRequest{
		Project:  gcpProj,
		Zone:     gcpZone,
		Instance: gcpInstanceName,
	})
	return err
}

func stopValheimServer(ctx context.Context, client *compute.InstancesClient) error {
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
