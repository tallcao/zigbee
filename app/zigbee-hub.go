package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"zigbee/models"
	"zigbee/services"
	"zigbee/utils"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/nats-io/nats.go"
)

type ZigbeeHub struct {
	mu sync.Mutex
	// savedAddrs []string

	// addr: uuid
	savedDevices map[string]string

	// name: addr
	receivedDevices map[string]string

	subscriptions []*nats.Subscription

	mqtt *services.MqttService
	nc   *nats.Conn
	db   *sql.DB
}

func NewZigbeeHub(mqtt *services.MqttService, nc *nats.Conn, db *sql.DB) (*ZigbeeHub, error) {
	if mqtt == nil {
		return nil, errors.New("mqtt service cannot be nil")
	}
	if nc == nil {
		return nil, errors.New("nats connection cannot be nil")
	}

	hub := &ZigbeeHub{

		savedDevices:    make(map[string]string),
		receivedDevices: make(map[string]string),

		subscriptions: make([]*nats.Subscription, 0),

		mqtt: mqtt,
		nc:   nc,
		db:   db,
	}

	return hub, nil
}

func (h *ZigbeeHub) getUUIDByName(name string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	addr, ok := h.receivedDevices[name]
	if !ok {
		return "", false
	}
	uuid, ok := h.savedDevices[addr]
	if !ok {
		return "", false
	}

	return uuid, true

}

func (h *ZigbeeHub) getAddrByUUID(uuid string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for addr, u := range h.savedDevices {
		if u == uuid {
			return addr, true
		}
	}

	return "", false

}

func (h *ZigbeeHub) syncDevices(payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var devices []models.ZigbeeBridgeDevice
	if err := json.Unmarshal(payload, &devices); err != nil {
		log.Printf("Failed to unmarshal payload: %v", err)
		return
	}

	receivedDevices := make(map[string]string)
	// Create a map of received devices for easy lookup
	receivedAddrs := make(map[string]models.ZigbeeBridgeDevice)
	for _, dev := range devices {

		if dev.Definition.Model == "" || dev.Definition.Vendor == "" {
			continue
		}

		if !dev.Disabled && dev.Type != "Coordinator" {

			receivedDevices[dev.Name] = dev.Addr

			topic := fmt.Sprintf("zigbee2mqtt/%v", dev.Name)
			h.mqtt.AddSubscriptionTopic(topic, 1, h.zigbeeDevicesHandler())

			topic = fmt.Sprintf("zigbee2mqtt/%v/availability", dev.Name)
			h.mqtt.AddSubscriptionTopic(topic, 1, h.zigbeeDevicesStateHandler())

			receivedAddrs[dev.Addr] = dev
		}
	}

	for name := range h.receivedDevices {
		if _, ok := receivedDevices[name]; !ok {
			topic := fmt.Sprintf("zigbee2mqtt/%v", name)
			h.mqtt.Unsubscribe(topic)

			topic = fmt.Sprintf("zigbee2mqtt/%v/availability", name)
			h.mqtt.Unsubscribe(topic)
		}
	}

	h.receivedDevices = receivedDevices

	// mqtt sub devices

	changed := false

	// Find devices to delete (exist in savedAddrs but not in received devices)
	var deleteAddrs []string
	for addr := range h.savedDevices {
		if _, exists := receivedAddrs[addr]; !exists {
			deleteAddrs = append(deleteAddrs, addr)
			changed = true
		}
	}

	// Find devices to add (exist in received devices but not in savedAddrs)
	var addDevices []models.ZigbeeBridgeDevice
	for addr, dev := range receivedAddrs {

		found := false

		for savedAddr := range h.savedDevices {

			if addr == savedAddr {
				found = true
			}
		}
		if !found {
			changed = true
			addDevices = append(addDevices, dev)
		}
	}

	// Delete devices that no longer exist
	for _, addr := range deleteAddrs {
		if err := DeleteZigbeeDevice(h.db, addr); err != nil {
			log.Printf("Failed to delete device %s: %v", addr, err)
			continue
		}
		// Remove from saved addresses
		delete(h.savedDevices, addr)

		log.Printf("Deleted device with addr: %s", addr)
	}

	// Add new devices
	for _, dev := range addDevices {
		device := ZigbeeDevice{
			Name:   dev.Name,
			Addr:   dev.Addr,
			Model:  dev.Definition.Model,
			Vendor: dev.Definition.Vendor,
		}

		newDevice, err := CreateZigbeeDevice(h.db, device)
		if err != nil {
			log.Printf("Failed to create device %s: %v", dev.Addr, err)
			continue
		}

		// Add to saved addresses
		h.savedDevices[dev.Addr] = newDevice.UUID.String()

		log.Printf("Added new device with addr: %s", dev.Addr)
	}

	if !changed {
		return
	}
	// NATS add subscribe
	for _, sub := range h.subscriptions {
		if err := sub.Unsubscribe(); err != nil {
			log.Printf("Failed to unsubscribe from %s: %v", sub.Subject, err)
		}
	}

	for _, uuid := range h.savedDevices {
		h.subShadowAndCommand(uuid)

	}

	h.nc.Publish("notify.zigbee.update", []byte{})

}
func (h *ZigbeeHub) natsHandler() nats.MsgHandler {
	return func(msg *nats.Msg) {

		deviceUUID := strings.Split(msg.Subject, ".")[1]

		var shadow models.Shadow
		if err := json.Unmarshal(msg.Data, &shadow); err != nil {
			log.Printf("ERROR: Unmarshal shadow data error: %v", err)
			return
		}

		// 检查是否有 delta 数据
		if len(shadow.State.Delta) == 0 {
			// log.Printf("No delta data found for device %s", deviceUUID)
			return
		}

		data, err := json.Marshal(shadow.State.Delta)
		if err != nil {
			log.Printf("ERROR: Marshal delta data error: %v", err)
			return
		}

		addr, ok := h.getAddrByUUID(deviceUUID)
		if !ok {
			log.Printf("ERROR: not found addr by uuid: %v", deviceUUID)
			return
		}
		topic := fmt.Sprintf("zigbee2mqtt/%v/set", addr)

		if err := h.mqtt.PublishMessage(topic, 1, false, data); err != nil {
			log.Printf("ERROR: Failed to publish to MQTT topic '%s': %v\n", topic, err)
		}

	}
}
func (h *ZigbeeHub) zigbeeDevicesHandler() mqtt.MessageHandler {
	return func(client mqtt.Client, msg mqtt.Message) {
		name := utils.GetTopicN(msg.Topic(), 1)

		uuid, ok := h.getUUIDByName(name)
		if !ok {
			log.Printf("not found uuid by name %v", name)
		}

		subject := fmt.Sprintf("shadow.%v.reported", uuid)
		var update struct {
			DeviceUUID string         `json:"device_uuid"`
			State      map[string]any `json:"state"`
		}
		update.DeviceUUID = uuid

		err := json.Unmarshal(msg.Payload(), &update.State)

		if err != nil {
			log.Printf("ERROR: Unmarshal state data error: %v", err)
			return
		}

		// action data

		if action, ok := update.State["action"]; ok {

			var tmp struct {
				DeviceUUID string `json:"device_uuid"`
				Timestamp  int64  `json:"timestamp"`

				Action any `json:"action"`
			}

			tmp.DeviceUUID = uuid
			tmp.Timestamp = time.Now().UnixMilli()
			tmp.Action = action

			subject := fmt.Sprintf("controller.%v.%v", uuid, action)

			data, err := json.Marshal(tmp)
			if err != nil {
				log.Printf("ERROR: Marshal action data error: %v", err)
				return
			}
			h.nc.Publish(subject, data)

			return
		}

		data, err := json.Marshal(update)
		if err != nil {
			log.Printf("ERROR: Marshal update data error: %v", err)
			return
		}
		if err := h.nc.Publish(subject, data); err != nil {
			log.Printf("ERROR: Failed to publish to NATS subject '%s': %v\n", subject, err)
		}
	}
}
func (h *ZigbeeHub) zigbeeDevicesStateHandler() mqtt.MessageHandler {
	return func(client mqtt.Client, msg mqtt.Message) {
		name := utils.GetTopicN(msg.Topic(), 1)

		uuid, ok := h.getUUIDByName(name)
		if !ok {
			log.Printf("not found uuid by name %v", name)
		}

		var state models.ConnectionState
		err := json.Unmarshal(msg.Payload(), &state)
		if err != nil {
			log.Printf("ERROR: Unmarshal connection state data error: %v", err)
			return
		}

		connected := strings.EqualFold(state.State, "online")

		var update struct {
			DeviceUUID string         `json:"device_uuid"`
			State      map[string]any `json:"state"`
		}

		update.State = map[string]any{
			"connected": connected,
		}
		update.DeviceUUID = uuid

		data, err := json.Marshal(update)
		if err != nil {
			log.Printf("ERROR: Marshal update data error: %v", err)
			return
		}

		subject := fmt.Sprintf("shadow.%v.reported", uuid)

		if err := h.nc.Publish(subject, data); err != nil {
			log.Printf("ERROR: Failed to publish to NATS subject '%s': %v\n", subject, err)
		}

	}
}

func (h *ZigbeeHub) zigbeeHubCommandHandler() mqtt.MessageHandler {
	return func(client mqtt.Client, msg mqtt.Message) {

		var cmd struct {
			DeviceUUID string `json:"device_uuid"`
			Command    string `json:"command"`
			Data       string `json:"data,omitempty"`
		}

		err := json.Unmarshal(msg.Payload(), &cmd)

		if err != nil {
			log.Printf("ERROR: Unmarshal command data error: %v", err)
			return
		}

		addr, ok := h.getAddrByUUID(cmd.DeviceUUID)
		if !ok {
			log.Printf("ERROR: not found addr by uuid: %v", cmd.DeviceUUID)
			return
		}
		topic := fmt.Sprintf("zigbee2mqtt/%v/set", addr)

		if err := h.mqtt.PublishMessage(topic, 0, false, cmd.Data); err != nil {
			log.Printf("ERROR: Failed to publish to MQTT topic '%s': %v\n", topic, err)
			return
		}

	}
}

func (h *ZigbeeHub) zigbeeHubDevicesHandler() mqtt.MessageHandler {
	return func(client mqtt.Client, msg mqtt.Message) {
		h.syncDevices(msg.Payload())
	}
}

func (h *ZigbeeHub) zigbeeHubStateHandler() mqtt.MessageHandler {
	return func(client mqtt.Client, msg mqtt.Message) {
		var state models.ConnectionState

		err := json.Unmarshal(msg.Payload(), &state)
		if err != nil {
			log.Printf("ERROR: Unmarshal connection state data error: %v", err)
			return
		}

		// connected := state.State == "online"

		// if !connected {

		// 	// publish devices offline

		// }

	}
}

func (h *ZigbeeHub) zigbeeHubResponseHandler() mqtt.MessageHandler {
	return func(client mqtt.Client, msg mqtt.Message) {

		var tmp struct {
			Suffix string `json:"suffix"`
			Data   []byte `json:"data"`
		}

		tmp.Data = msg.Payload()

		parts := strings.Split(msg.Topic(), "/")

		if len(parts) >= 4 {

			tmp.Suffix = strings.Join(parts[3:], "/")

		} else {
			return
		}

		data, err := json.Marshal(tmp)

		if err == nil {
			h.nc.Publish("response.zigbee.bridge", data)

		}

	}
}

func (h *ZigbeeHub) subShadowAndCommand(uuid string) {

	subject := fmt.Sprintf("shadow.%v", uuid)
	sub, err := h.nc.Subscribe(subject, h.natsHandler())
	if err != nil {
		log.Printf("NATS subscribe error: %v", err)
	}
	h.subscriptions = append(h.subscriptions, sub)

	topic := fmt.Sprintf("commands/%v", uuid)
	h.mqtt.AddSubscriptionTopic(topic, 0, h.zigbeeHubCommandHandler())
}

func (h *ZigbeeHub) init() {
	devices, err := GetAllZigbeeDeviceAddrs(h.db)
	if err != nil {
		log.Printf("Failed to load zigbee devices: %v", err)
		return
	}

	for _, dev := range devices {
		uuid := dev.UUID.String()
		h.savedDevices[dev.Addr] = uuid
		h.subShadowAndCommand(uuid)

	}

}

// func sendErrorResponse(nc *nats.Conn, replySubject string, msg string, err error) {
// 	errMsg := fmt.Sprintf("%s: %v", msg, err)
// 	resp := models.NatsResponsePayload{
// 		Status:  "error",
// 		Message: errMsg,
// 	}
// 	respBytes, _ := json.Marshal(resp) // 忽略这里的序列化错误，因为是错误处理的错误
// 	nc.Publish(replySubject, respBytes)
// 	log.Printf("发送错误响应到 %s: %s\n", replySubject, errMsg)
// }

func (h *ZigbeeHub) Start() {
	h.init()

	// c.Subscribe(fmt.Sprintf("zigbee2mqtt/%s", dev.Name), 0, n.deviceDataCallback)
	// c.Subscribe(fmt.Sprintf("zigbee2mqtt/%s/availability", dev.Name), 0, n.deviceStateCallback)

	h.mqtt.AddSubscriptionTopic("zigbee2mqtt/bridge/devices", 1, h.zigbeeHubDevicesHandler())
	h.mqtt.AddSubscriptionTopic("zigbee2mqtt/bridge/state", 1, h.zigbeeHubStateHandler())

	h.mqtt.AddSubscriptionTopic("zigbee2mqtt/bridge/response/+", 1, h.zigbeeHubResponseHandler())

	// zigbee permit join
	h.nc.Subscribe("request.zigbee.permit-join", func(m *nats.Msg) {

		var req models.NatsRequestPayload
		err := json.Unmarshal(m.Data, &req)
		if err != nil {
			log.Printf("zigbee permit join json data error: %v", err)
			return
		}

		err = h.mqtt.PublishMessage("zigbee2mqtt/bridge/request/permit_join", 1, false, req.Data)
		if err != nil {
			log.Printf("zigbee permit join mqtt error: %v", err)
			return
		}

		// var resp models.NatsResponsePayload

		// resp.Status = "success"
		// resp.Message = "zigbee2mqtt perimit join starting..."
		// respBytes, err := json.Marshal(resp)
		// if err != nil {
		// 	sendErrorResponse(h.nc, m.Reply, "序列化响应失败", err)
		// 	return
		// }
		// if err := h.nc.Publish(m.Reply, respBytes); err != nil {
		// 	log.Printf("发送响应失败: %v\n", err)
		// }
	})

	// zigbee bridge request
	h.nc.Subscribe("request.zigbee.bridge", func(m *nats.Msg) {

		var req struct {
			Suffix string `json:"suffix"`
			Data   []byte `json:"data"`
		}

		err := json.Unmarshal(m.Data, &req)
		if err != nil {
			log.Printf("zigbee bridge request json data error: %v", err)

			return
		}

		topic := fmt.Sprintf("zigbee2mqtt/bridge/request/%v", req.Suffix)
		err = h.mqtt.PublishMessage(topic, 1, false, req.Data)
		if err != nil {
			log.Printf("zigbee bridge request mqtt error: %v", err)

			return
		}

	})

}
