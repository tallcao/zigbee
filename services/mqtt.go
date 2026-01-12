package services

import (
	"fmt"
	"log"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var (
	DefaultMqttService *MqttService
	once               sync.Once
)

type MqttService struct {
	id string

	client mqtt.Client

	mu       sync.Mutex                     // 保护订阅主题列表
	topics   map[string]byte                // 存储要订阅的主题及其 QoS
	handlers map[string]mqtt.MessageHandler // 存储特定主题的处理器
	running  bool                           // 服务是否正在运行
}

func InitMqttService(id, brokerURL, user, password string) error {
	var initErr error
	once.Do(func() {
		DefaultMqttService = NewMqttService(id, brokerURL, user, password)
		initErr = DefaultMqttService.Start()
	})
	return initErr
}

// GetMqttService returns the default SparkplugService instance
func GetMqttService() *MqttService {
	return DefaultMqttService
}

// NewMqttClientService 创建一个新的 MQTT 客户端服务实例
func NewMqttService(id, brokerURL, user, password string) *MqttService {

	opts := mqtt.NewClientOptions().AddBroker(brokerURL).SetClientID(id).SetOrderMatters(false)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetConnectRetry(true) // 启用自动重连

	opts.SetUsername(user)
	opts.SetPassword(password)

	service := &MqttService{
		id:       id,
		topics:   make(map[string]byte),
		handlers: make(map[string]mqtt.MessageHandler),
	}

	// 设置 OnConnect 回调
	opts.SetOnConnectHandler(service.onConnectHandler)

	service.client = mqtt.NewClient(opts)

	return service
}

// 这个方法可以在服务启动前或运行中调用
func (s *MqttService) AddSubscriptionTopic(topic string, qos byte, handler mqtt.MessageHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.topics[topic] = qos
	s.handlers[topic] = handler

	if s.running && s.client.IsConnected() {
		s.subscribeToTopic(topic, qos)
	}
}

// subscribeToTopic 内部函数，执行单个主题的订阅
func (s *MqttService) subscribeToTopic(topic string, qos byte) {
	handler, exists := s.handlers[topic]
	if !exists {
		log.Printf("ERROR: No handler registered for topic '%s'\n", topic)
		return
	}

	token := s.client.Subscribe(topic, qos, handler)
	token.Wait()
	if token.Error() != nil {
		log.Printf("ERROR: Failed to subscribe to topic '%s': %v\n", topic, token.Error())
	} else {
		log.Printf("Subscribed to topic: '%s' (QoS %d)\n", topic, qos)
	}
}

// onConnectHandler 是 MQTT 客户端连接成功时的回调
func (s *MqttService) onConnectHandler(client mqtt.Client) {
	log.Println("MQTT Client Connected!")
	s.mu.Lock()
	defer s.mu.Unlock()

	// 在连接成功时，订阅所有已注册的主题
	if len(s.topics) > 0 {
		log.Printf("Resubscribing to %d topics...\n", len(s.topics))
		for topic, qos := range s.topics {
			s.subscribeToTopic(topic, qos)
		}
	} else {
		log.Println("No topics registered for subscription.")
	}

}

func (s *MqttService) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return fmt.Errorf("MQTT Client Service is already running")
	}
	log.Println("Starting MQTT Client Service...")

	if token := s.client.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to connect MQTT client: %w", token.Error())
	}
	s.running = true
	log.Println("MQTT Client Service started.")
	return nil
}

func (s *MqttService) Stop() {
	log.Println("Stopping MQTT Client Service...")
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
	if s.client.IsConnected() {
		s.client.Disconnect(250)
	}
	log.Println("MQTT Client Service stopped.")
}

func (s *MqttService) PublishMessage(topic string, qos byte, retained bool, payload interface{}) error {
	if !s.client.IsConnected() {
		return fmt.Errorf("MQTT client not connected, cannot publish")
	}
	token := s.client.Publish(topic, qos, retained, payload)
	token.Wait()
	if token.Error() != nil {
		return fmt.Errorf("failed to publish message to topic '%s': %w", topic, token.Error())
	}
	log.Printf("Published message to topic: %s\n", topic)
	return nil
}

func (s *MqttService) GetClient() mqtt.Client {
	return s.client
}

func (s *MqttService) Unsubscribe(topic string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if topic exists in our subscriptions
	if _, exists := s.topics[topic]; !exists {
		return fmt.Errorf("topic '%s' is not subscribed", topic)
	}

	// If client is connected, unsubscribe from broker
	if s.running && s.client.IsConnected() {
		token := s.client.Unsubscribe(topic)
		token.Wait()
		if token.Error() != nil {
			return fmt.Errorf("failed to unsubscribe from topic '%s': %w", topic, token.Error())
		}
		log.Printf("Unsubscribed from topic: %s\n", topic)
	}

	// Remove topic from internal maps
	delete(s.topics, topic)
	delete(s.handlers, topic)

	return nil
}
