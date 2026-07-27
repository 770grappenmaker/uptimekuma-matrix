package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type Heartbeat struct {
	MonitorID      int    `json:"monitorID"`
	Status         int    `json:"status"`
	Time           string `json:"time"`
	Msg            string `json:"msg"`
	Ping           int    `json:"ping"`
	Important      bool   `json:"important"`
	Retries        int    `json:"retries"`
	Timezone       string `json:"timezone"`
	TimezoneOffset string `json:"timezoneOffset"`
	LocalDateTime  string `json:"localDateTime"`
	LastDownTime   string `json:"lastDownTime"`
}

type Monitor struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	PathName    string `json:"pathName"`
	URL         string `json:"url"`
}

type HeartbeatEvent struct {
	Message   string    `json:"msg"`
	Monitor   Monitor   `json:"monitor"`
	Heartbeat Heartbeat `json:"heartbeat"`
}

func getSingleHeader(header string, w http.ResponseWriter, r *http.Request) *string {
	var hdr = r.Header[header]

	if hdr == nil {
		http.Error(w, fmt.Sprintf("Missing %s header", header), http.StatusBadRequest)
		return nil
	}

	if len(hdr) != 1 {
		http.Error(w, fmt.Sprintf("%s header expects a single value!", header), http.StatusBadRequest)
		return nil
	}

	return &hdr[0]
}

func getOptionalHeader(header string, deflt string, w http.ResponseWriter, r *http.Request) *string {
	var hdr = r.Header[header]

	if hdr == nil {
		return &deflt
	}

	if len(hdr) != 1 {
		http.Error(w, fmt.Sprintf("%s header expects a single value!", header), http.StatusBadRequest)
		return nil
	}

	return &hdr[0]
}

func doTemplate(messageTemplate string, event HeartbeatEvent) (string, error) {
	tmpl, err := template.New("message").Parse(messageTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer

	err = tmpl.Execute(&buf, event)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	csapi := getSingleHeader("X-Matrix-Csapi", w, r)
	if csapi == nil {
		return
	}

	matrixRoom := getSingleHeader("X-Matrix-Room", w, r)
	if matrixRoom == nil {
		return
	}

	matrixToken := getSingleHeader("X-Matrix-Token", w, r)
	if matrixToken == nil {
		return
	}

	matrixID := getSingleHeader("X-Matrix-Id", w, r)
	if matrixID == nil {
		return
	}

	messageType := getOptionalHeader("X-Matrix-Type", "m.notice", w, r)
	if messageType == nil {
		return
	}

	messageTemplate := getOptionalHeader("X-Matrix-Template", "{{.Message}}", w, r)
	if messageTemplate == nil {
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	defer r.Body.Close()

	var payload HeartbeatEvent
	err = json.Unmarshal(body, &payload)

	if err != nil {
		http.Error(w, "Invalid Webhook body", http.StatusBadRequest)
		return
	}

	var client *mautrix.Client
	client, err = mautrix.NewClient(*csapi, id.UserID(*matrixID), *matrixToken)

	if err != nil {
		http.Error(w, "Failed to reach homeserver", http.StatusBadGateway)
		return
	}

	var messageBody string
	messageBody, err = doTemplate(*messageTemplate, payload)

	_, err = client.SendMessageEvent(context.Background(), id.RoomID(*matrixRoom), event.EventMessage, &event.MessageEventContent{
		MsgType: event.MessageType(*messageType),
		Body:    messageBody,
	})

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to deliver message to homeserver: %s", err), http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func main() {
	port := flag.Int("port", 1234, "the port to listen on")
	addr := flag.String("addr", "0.0.0.0", "the address to bind to")
	help := flag.Bool("help", false, "shows this help")
	flag.Parse()

	if *help {
		flag.PrintDefaults()
		os.Exit(2)
	}

	http.HandleFunc("/", handler)
	fmt.Printf("Server listening on %s:%d...\n", *addr, *port)
	http.ListenAndServe(fmt.Sprintf("%s:%d", *addr, *port), nil)
}
