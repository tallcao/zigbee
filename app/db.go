package app

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// Event types
const (
	ZigbeeDeviceCreated = "zigbee.device.created"
	ZigbeeDeviceDeleted = "zigbee.device.deleted"
)

type ZigbeeDevice struct {
	UUID   uuid.UUID
	Name   string
	Addr   string
	Model  string
	Vendor string
}

func CreateZigbeeDevice(db *sql.DB, device ZigbeeDevice) (*ZigbeeDevice, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	device.UUID = uuid.New()
	deviceQuery := `
        INSERT INTO devices (uuid, name,  connection_type, vendor, model)
        VALUES ($1, $2, $3, $4,$5)`

	_, err = tx.Exec(deviceQuery, device.UUID, device.Name, "zigbee", device.Vendor, device.Model)
	if err != nil {
		return nil, fmt.Errorf("failed to insert device: %w", err)
	}

	// 4. Insert into zigbee_devices table
	zigbeeQuery := `
        INSERT INTO zigbee_devices (uuid, addr, device_uuid)
        VALUES ($1, $2, $3)`

	zigbeeUUID := uuid.New()
	_, err = tx.Exec(zigbeeQuery, zigbeeUUID, device.Addr, device.UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to insert zigbee device: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Publish event to NATS
	// eventData := map[string]any{
	// 	"addr":        device.Addr,
	// 	"device_uuid": device.UUID,
	// }
	// jsonData, _ := json.Marshal(eventData)
	// if err := services.NC.Publish(ZigbeeDeviceCreated, jsonData); err != nil {
	// 	log.Printf("Failed to publish zigbee device created event: %v", err)
	// }

	return &device, nil
}

func DeleteZigbeeDevice(db *sql.DB, addr string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// First get the device_uuid from zigbee_devices
	var deviceUUID string
	getUUIDQuery := `
        SELECT device_uuid 
        FROM zigbee_devices 
        WHERE addr = $1`

	err = tx.QueryRow(getUUIDQuery, addr).Scan(&deviceUUID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("zigbee device with addr %s not found", addr)
		}
		return fmt.Errorf("failed to get device UUID: %w", err)
	}

	// Delete from zigbee_devices first (child table)
	deleteZigbeeQuery := `
        DELETE FROM zigbee_devices 
        WHERE addr = $1`

	_, err = tx.Exec(deleteZigbeeQuery, addr)
	if err != nil {
		return fmt.Errorf("failed to delete zigbee device: %w", err)
	}

	// Delete from devices table (parent table)
	deleteDeviceQuery := `
        DELETE FROM devices 
        WHERE uuid = $1`

	_, err = tx.Exec(deleteDeviceQuery, deviceUUID)
	if err != nil {
		return fmt.Errorf("failed to delete device: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Publish event to NATS
	// eventData := map[string]any{
	// 	"addr":        addr,
	// 	"device_uuid": deviceUUID,
	// }
	// jsonData, _ := json.Marshal(eventData)

	// if err := services.NC.Publish(ZigbeeDeviceDeleted, jsonData); err != nil {
	// 	log.Printf("Failed to publish zigbee device deleted event: %v", err)
	// }

	return nil
}

func GetAllZigbeeDeviceAddrs(db *sql.DB) ([]*ZigbeeDevice, error) {
	query := `SELECT  addr, device_uuid FROM zigbee_devices`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*ZigbeeDevice

	for rows.Next() {
		var device ZigbeeDevice
		if err := rows.Scan(&device.Addr, &device.UUID); err != nil {
			return nil, fmt.Errorf("failed to scan addr: %w", err)
		}
		result = append(result, &device)
	}

	return result, rows.Err()

}
