package agent

import (
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/neandrson/go_calc_final/internal/config"
	"github.com/neandrson/go_calc_final/internal/models"
	"github.com/neandrson/go_calc_final/internal/repositories/queue"
)

type IAgent interface {
	// Start запускает агента
	Start()
	// CalculateExpression считает subexpression
	CalculateExpression(task *models.SubExpression)
	// StartHeartbeats отправка heartbeats
	StartHeartbeats()
}

type Agent struct {
	id                         string
	expressionQueueRepository  queue.Repository
	calculationQueueRepository queue.Repository
	heartbeatQueueRepository   queue.Repository
	rpcQueueRepository         queue.Repository
	calculationTimeouts        config.CalculationTimeoutsConfig
}

func NewAgent(expressionQueueRepo, calculationQueueRepo, heartbeatQueueRepo, rpcQueueRepo queue.Repository,
	timeouts config.CalculationTimeoutsConfig) *Agent {
	id := uuid.NewString()
	return &Agent{
		id:                         id,
		expressionQueueRepository:  expressionQueueRepo,
		calculationQueueRepository: calculationQueueRepo,
		heartbeatQueueRepository:   heartbeatQueueRepo,
		rpcQueueRepository:         rpcQueueRepo,
		calculationTimeouts:        timeouts,
	}
}

func (a *Agent) Start() {
	// соединение с очередью subexpressions
	err := a.expressionQueueRepository.Connect()
	if err != nil {
		log.Fatalf("Не удалось подключиться к очереди: %v", err)
	}
	defer a.expressionQueueRepository.Close()

	tasks, err := a.expressionQueueRepository.Consume()
	if err != nil {
		log.Fatalf("Не удалось использовать задачи из очереди: %v", err)
	}

	// начинаем посылать heartbeats
	go a.StartHeartbeats()

	// обработка subexpressions из очереди
	for task := range tasks {
		expressionStruct := &models.SubExpression{}
		_ = json.Unmarshal(task, expressionStruct)

		// формирование ответа оркестратору в том, что взяли subexpression на обработку
		idAgent, _ := uuid.Parse(a.id)
		rpcAnswer := models.RPCAnswer{
			IdSubExpression: expressionStruct.Id,
			IdAgent:         idAgent,
		}
		err := a.rpcQueueRepository.Connect()
		if err != nil {
			log.Printf("ошибка подключения к очереди rpc")
		}
		rpcJson, err := json.Marshal(rpcAnswer)
		if err != nil {
			log.Printf("ошибка unmarshal rpc")
		}
		err = a.rpcQueueRepository.Publish(rpcJson)
		if err != nil {
			log.Printf("ошибка публикации rpc")
		}
		a.rpcQueueRepository.Close()

		// подсчет subexpression
		a.CalculateExpression(expressionStruct)
	}
}

func (a *Agent) CalculateExpression(task *models.SubExpression) {
	result, err := Calculate(task, a.calculationTimeouts)
	if err != nil {
		task.Error = true
	}
	task.Result = result

	err = a.calculationQueueRepository.Connect()
	if err != nil {
		log.Fatalf("Не удалось подключиться к очереди: %v", err)
	}
	defer a.calculationQueueRepository.Close()

	expressionJson, _ := json.Marshal(task)
	err = a.calculationQueueRepository.Publish(expressionJson)
	if err != nil {
		log.Printf("Не удалось опубликовать выполненную задачу в очереди: %v", err)
	}
}

func (a *Agent) StartHeartbeats() {
	// Открываем соединение один раз, а не на каждую итерацию
	err := a.heartbeatQueueRepository.Connect()
	if err != nil {
		log.Fatalf("Не удалось подключиться к очереди: %v\n", err)
	}
	defer a.heartbeatQueueRepository.Close() // Закрыть соединение при завершении функции

	ticker := time.NewTicker(time.Second * 3)
	defer ticker.Stop() // Остановить тикер, когда функция завершится

	for range ticker.C {
		agent := &models.Agent{
			Id: a.id,
		}
		agentJson, err := json.Marshal(agent)
		if err != nil {
			log.Printf("Не удалось закодировать агента: %v\n", err)
			continue // Продолжить цикл, если кодирование не удалось
		}

		err = a.heartbeatQueueRepository.Publish(agentJson)
		if err != nil {
			log.Printf("Не удалось опубликовать задачу в очереди: %v", err)
		}
	}
}
