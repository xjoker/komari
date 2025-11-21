package tasks

import (
	"time"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/utils"
	"gorm.io/gorm"
)

func AddPingTask(clients []string, name string, target, task_type string, interval int) (uint, error) {
	// Fast timeout for simple single-record insert
	db := dbcore.WithFastTimeout(dbcore.GetDBInstance())
	task := models.PingTask{
		Clients:  clients,
		Name:     name,
		Type:     task_type,
		Target:   target,
		Interval: interval,
	}
	if err := db.Create(&task).Error; err != nil {
		return 0, err
	}
	ReloadPingSchedule()
	return task.Id, nil
}

func DeletePingTask(id []uint) error {
	// Default timeout for DELETE with IN clause
	db := dbcore.WithDefaultTimeout(dbcore.GetDBInstance())
	result := db.Where("id IN ?", id).Delete(&models.PingTask{})
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	ReloadPingSchedule()
	return result.Error
}

func EditPingTask(tasks []*models.PingTask) error {
	// Default timeout for multiple UPDATE operations
	db := dbcore.WithDefaultTimeout(dbcore.GetDBInstance())
	for _, task := range tasks {
		result := db.Model(&models.PingTask{}).Where("id = ?", task.Id).Updates(task)
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
	}
	ReloadPingSchedule()
	return nil
}

func GetAllPingTasks() ([]models.PingTask, error) {
	// Default timeout for fetching all ping tasks
	db := dbcore.WithDefaultTimeout(dbcore.GetDBInstance())
	var tasks []models.PingTask
	if err := db.Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func SavePingRecord(record models.PingRecord) error {
	// Fast timeout for high-frequency single-record insert
	db := dbcore.WithFastTimeout(dbcore.GetDBInstance())
	return db.Create(&record).Error
}

func DeletePingRecordsBefore(time time.Time) error {
	// Slow timeout for time-range DELETE that could affect many rows
	db := dbcore.WithSlowTimeout(dbcore.GetDBInstance())
	err := db.Where("time < ?", time).Delete(&models.PingRecord{}).Error
	return err
}

func DeletePingRecords(id []uint) error {
	// Default timeout for DELETE with IN clause
	db := dbcore.WithDefaultTimeout(dbcore.GetDBInstance())
	result := db.Where("task_id IN ?", id).Delete(&models.PingRecord{})
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return result.Error
}

func DeleteAllPingRecords() error {
	// Slow timeout for DELETE all records
	db := dbcore.WithSlowTimeout(dbcore.GetDBInstance())
	result := db.Exec("DELETE FROM ping_records")
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return result.Error
}

func ReloadPingSchedule() error {
	// Default timeout for fetching all ping tasks
	db := dbcore.WithDefaultTimeout(dbcore.GetDBInstance())
	var pingTasks []models.PingTask
	if err := db.Find(&pingTasks).Error; err != nil {
		return err
	}
	return utils.ReloadPingSchedule(pingTasks)
}

func GetPingRecords(uuid string, taskId int, start, end time.Time) ([]models.PingRecord, error) {
	// Default timeout for filtered SELECT query
	db := dbcore.WithDefaultTimeout(dbcore.GetDBInstance())
	var records []models.PingRecord
	dbQuery := db.Model(&models.PingRecord{})
	if uuid != "" {
		dbQuery = dbQuery.Where("client = ?", uuid)
	}
	if taskId >= 0 {
		dbQuery = dbQuery.Where("task_id = ?", uint(taskId))
	}
	if err := dbQuery.Where("time >= ? AND time <= ?", start, end).Order("time DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}
