package queue

import (
	"errors"
)

var ErrQueueNotConnected = errors.New("очередь не подключена")

type Repository interface {
	// Connect осуществляет соединение с очередью с именем queueName
	Connect() error
	// Close закрывает соединение с очередью
	Close() error
	// Publish публикует запись в очередь
	Publish([]byte) error
	// Consume возвращает канал, откуда можно читать записи с очереди
	Consume() (<-chan []byte, error)
}
