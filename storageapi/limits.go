package storageapi

import (
	"context"
	"sync"
	"time"
)

// Из-за подсчёта байтов в секунду,
// к сожалению, не получается оформить в виде простой обёртки над http.Handler,
// которая была бы возможна в случае одного лишь RPS.
// Также, тогда было бы целесообразно воспользоваться "golang.org/x/time/rate".

const (
	UploadOperationIndex = iota
	DownloadOperationIndex
	DeleteOperationIndex
	InfoOperationIndex
)

// ClientOperationKey ключ для хранилища текущих операций - IP-адрес пользователя и константа, указывающая на операцию.
type ClientOperationKey struct {
	IP        string // IP-адрес отправившего запрос.
	Operation int    // Константное значение, соответствующее операции, к примеру - UploadOperationIndex.
}

// RequestTicket тикет с временной меткой запроса и размером данных.
type RequestTicket struct {
	BytesLength int       // Размер файла запроса в байтах (для всех операций, кроме "инфо").
	Timestamp   time.Time // Время выполнения запроса.
}

// OperationTickets структура с тикетами для конкретной операции конкретного пользователя.
type OperationTickets struct {
	mu      sync.Mutex
	tickets []RequestTicket
}

// IsRequestAllowed проверяет, доступна ли пользователю операция по требованиям лимитов, а так же добавляет учёт нового запроса в текущую статистику.
func (fs *FileOperationsServer) IsRequestAllowed(key ClientOperationKey, dataLength int) (rpsLimited bool, bpsLimited bool) {
	if fs.BPSLimit == 0 && fs.RPSLimit == 0 {
		return false, false
	}

	// получение текущей статистики для определённой операции конкретного пользователя
	operationValue, ok := fs.currentOperations.Load(key)
	if !ok {
		tickets := []RequestTicket{
			{BytesLength: dataLength, Timestamp: time.Now()},
		}
		fs.currentOperations.Store(key, &OperationTickets{tickets: tickets})
		return false, false
	}
	operation := operationValue.(*OperationTickets)

	// подсчёт RPS, BPS и проверка на превышение лимитов
	rps := 0
	bps := 0
	lastActualTicket := 0
	now := time.Now()
	intervalStart := now.Add(-time.Second)

	operation.mu.Lock()
	defer operation.mu.Unlock()
	for i, ticket := range operation.tickets {
		if intervalStart.Before(ticket.Timestamp) {
			rps++
			bps += ticket.BytesLength
			lastActualTicket = i
		}
	}

	if fs.RPSLimit > 0 && rps+1 > fs.RPSLimit {
		return true, false
	} else if fs.BPSLimit > 0 && bps+dataLength > fs.BPSLimit {
		return false, true
	}

	// обрезание устаревших тикетов и добавление нового
	operation.tickets = append(operation.tickets[lastActualTicket:], RequestTicket{
		BytesLength: dataLength,
		Timestamp:   now,
	})
	fs.currentOperations.Store(key, operation)
	return false, false
}

// StartTicketsCleaner периодически удаляет устаревшие тикеты.
func (fs *FileOperationsServer) StartTicketsCleaner(ctx context.Context) {
	for {
		// завершение проверки через контекст
		if ctx.Err() != nil {
			return
		}
		// длительность паузы выбрана "на глаз", можно изменить
		time.Sleep(10 * time.Minute)
		fs.currentOperations.Range(func(key, value any) bool {
			operation := value.(*OperationTickets)
			firstActualTicket := len(operation.tickets)
			// определение последнего актуального тикета
			now := time.Now()
			intervalStart := now.Add(-time.Second)
			operation.mu.Lock()
			defer operation.mu.Unlock()
			for i, ticket := range operation.tickets {
				if intervalStart.Before(ticket.Timestamp) {
					firstActualTicket = i
					break
				}
			}
			// ситуация, когда все тикеты устарели
			if firstActualTicket == len(operation.tickets) {
				fs.currentOperations.Delete(key)
				return true
			}
			// обрезание до актуального тикета
			operation.tickets = operation.tickets[firstActualTicket:]
			fs.currentOperations.Store(key, operation)
			return true
		})
	}
}
