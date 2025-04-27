package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/neandrson/go_calc_final/internal/models"
	"github.com/neandrson/go_calc_final/internal/repositories/agent"
	"github.com/neandrson/go_calc_final/internal/repositories/expression"
	"github.com/neandrson/go_calc_final/internal/repositories/queue"
	"github.com/neandrson/go_calc_final/internal/repositories/subExpression"
	orchestratorutils "github.com/neandrson/go_calc_final/internal/services/orchestrator/utils"
)

type IOrchestrator interface {
	CreateExpression(ctx context.Context, expression, idempotencyKey, userId string) (error, string)
	GetExpressions(ctx context.Context, userId string) ([]*models.Expression, error)
	GetSubExpressions(ctx context.Context) ([]*models.SubExpression, error)
	GetExpression(ctx context.Context, id, userId string) (*models.Expression, error)
	GetExpressionByKey(ctx context.Context, key, userId string) (*models.Expression, error)
	UpdateExpressionState(ctx context.Context, key string, state models.ExpressionState) error
	// ReceiveHeartbeats принимает heartbeats из очереди от агента
	ReceiveHeartbeats()
	// ReceiveCalculations принимает подсчитанные subexpression из очереди от агента
	ReceiveCalculations(ctx context.Context)
	CreateAgentIfNotExists(id string)
	GetAgents() ([]*models.Agent, error)
	// SendSubExpression отправляет subexpressions в очередь, которые могут подсчитаться (являются независимыми от ответов других subexpressions)
	SendSubExpression()
	// ReceiveRPCTasks принимает ответы от агента о том, какой subexpression он взял на обработку
	ReceiveRPCTasks(ctx context.Context)
	// RetrySubExpressions переназначает неподсчитанные subexpressions умершего агента на другого
	RetrySubExpressions(ctx context.Context)
}

type Orchestrator struct {
	expressionRepository        expression.Repository
	subExpressionRepository     subExpression.Repository
	agentRepository             agent.Repository
	expressionsQueueRepository  queue.Repository
	calculationsQueueRepository queue.Repository
	heartbeatsQueueRepository   queue.Repository
	rpcQueueRepository          queue.Repository
	retrySubExpressionTimout    time.Duration
}

func NewOrchestrator(ctx context.Context, expressionRepo expression.Repository,
	subExpressionRepo subExpression.Repository,
	expressionsQueueRepo queue.Repository,
	calculationsQueueRepository queue.Repository,
	heartbeatsQueueRepository queue.Repository,
	rpcQueueRepository queue.Repository,
	agentRepo agent.Repository,
	retrySubExpressionTimout time.Duration) *Orchestrator {
	orch := &Orchestrator{
		expressionRepository:        expressionRepo,
		subExpressionRepository:     subExpressionRepo,
		agentRepository:             agentRepo,
		expressionsQueueRepository:  expressionsQueueRepo,
		calculationsQueueRepository: calculationsQueueRepository,
		heartbeatsQueueRepository:   heartbeatsQueueRepository,
		rpcQueueRepository:          rpcQueueRepository,
		retrySubExpressionTimout:    retrySubExpressionTimout,
	}
	go orch.SendSubExpression()
	go orch.ReceiveHeartbeats()
	go orch.ReceiveCalculations(ctx)
	go orch.ReceiveRPCTasks(ctx)
	go orch.RetrySubExpressions(ctx)
	return orch
}

func (o *Orchestrator) CreateExpression(
	ctx context.Context,
	expression,
	idempotencyKey,
	userId string,
) (error, string) {
	createdExpression, err := o.expressionRepository.CreateExpression(ctx, expression, idempotencyKey, userId)
	if err != nil {
		return err, ""
	}
	_, err = orchestratorutils.SplitToSubtasks(ctx, createdExpression, o.subExpressionRepository)
	if err != nil {
		exprId, _ := uuid.Parse(createdExpression.Id)
		o.subExpressionRepository.DeleteSubExpressionsByExpressionId(ctx, exprId)
		o.expressionRepository.DeleteExpressionById(ctx, exprId)
		return fmt.Errorf("ошибка разделена на подзадачи: %e", err), ""
	}
	return nil, createdExpression.Id
}

func (o *Orchestrator) GetExpressions(ctx context.Context, userId string) ([]*models.Expression, error) {
	return o.expressionRepository.GetExpressions(ctx, userId)
}

func (o *Orchestrator) GetSubExpressions(ctx context.Context) ([]*models.SubExpression, error) {
	return o.subExpressionRepository.GetSubExpressionsList(ctx)
}

func (o *Orchestrator) GetExpression(ctx context.Context, id, userId string) (*models.Expression, error) {
	return o.expressionRepository.GetExpressionById(ctx, id, userId)
}

func (o *Orchestrator) GetExpressionByKey(ctx context.Context, key, userId string) (*models.Expression, error) {
	return o.expressionRepository.GetExpressionByKey(ctx, key, userId)
}

func (o *Orchestrator) UpdateExpressionState(ctx context.Context, key string, state models.ExpressionState) error {
	return o.expressionRepository.UpdateState(ctx, key, state)
}

func (o *Orchestrator) ReceiveHeartbeats() {
	err := o.heartbeatsQueueRepository.Connect()
	if err != nil {
		log.Fatalf("Не удалось подключиться к очереди: %v", err)
	}
	defer o.heartbeatsQueueRepository.Close()

	heartbeats, err := o.heartbeatsQueueRepository.Consume()
	if err != nil {
		log.Printf("Не удалось использовать задачи из очереди: %v", err)
	}
	for heartbeat := range heartbeats {
		agent := models.Agent{}
		err = json.Unmarshal(heartbeat, &agent)
		if err != nil {
			log.Printf("Не удалось декодировать агента: %v", err)
			continue
		}
		o.CreateAgentIfNotExists(agent.Id)
	}
}

func (o *Orchestrator) ReceiveCalculations(ctx context.Context) {
	err := o.calculationsQueueRepository.Connect()
	if err != nil {
		log.Fatalf("Не удалось подключиться к очереди: %v", err)
	}
	defer o.calculationsQueueRepository.Close()

	finishedTasks, err := o.calculationsQueueRepository.Consume()
	if err != nil {
		log.Printf("Не удалось использовать задачи из очереди: %v", err)
	}
	for task := range finishedTasks {
		expressionStruct := &models.SubExpression{}
		err = json.Unmarshal(task, expressionStruct)
		if err != nil {
			log.Printf("ошибка unmarshal подвыражение: %e", err)
		}
		if expressionStruct.Error {
			err = o.subExpressionRepository.DeleteSubExpressionsByExpressionId(ctx, expressionStruct.ExpressionId)
			if err != nil {
				log.Printf("ошибка удаления подвыражений: %e", err)
			}
			err = o.expressionRepository.UpdateState(ctx, expressionStruct.ExpressionId.String(), models.ExpressionError)
			if err != nil {
				log.Printf("ошибка обновления состояния: %e", err)
			}
			continue
		}
		err = o.subExpressionRepository.UpdateSubExpressions(ctx, expressionStruct)
		if err != nil {
			log.Printf("ошибка обновления подвыражения: %e", err)
		}
		if expressionStruct.IsLast {
			err = o.expressionRepository.UpdateExpressionById(ctx, expressionStruct.ExpressionId, expressionStruct.Result)
			if err != nil {
				log.Printf("ошибка обновления выражения: %e", err)
			}
			err = o.subExpressionRepository.DeleteSubExpressionsByExpressionId(ctx, expressionStruct.ExpressionId)
			if err != nil {
				log.Printf("ошибка удаления подвыражений: %e", err)
			}
		}
	}

}
func (o *Orchestrator) CreateAgentIfNotExists(id string) {
	_ = o.agentRepository.CreateIfNotExistsAndUpdateHeartbeat(id)
}

func (o *Orchestrator) GetAgents() ([]*models.Agent, error) {
	return o.agentRepository.GetAgents()
}

func (o *Orchestrator) SendSubExpression() {
	listener := o.subExpressionRepository.GetSubExpressions()
	for subExpr := range listener {
		err := o.expressionsQueueRepository.Connect()
		if err != nil {
			log.Printf("")
		}
		expressionJson, err := json.Marshal(subExpr)
		if err != nil {
			log.Printf("")
		}
		err = o.expressionsQueueRepository.Publish(expressionJson)
		if err != nil {
			log.Printf("")
		}
		o.expressionsQueueRepository.Close()
	}
}
func (o *Orchestrator) ReceiveRPCTasks(ctx context.Context) {
	err := o.rpcQueueRepository.Connect()
	if err != nil {
		log.Fatalf("Не удалось подключиться к очереди: %v", err)
	}
	defer o.rpcQueueRepository.Close()

	rpcTasks, err := o.rpcQueueRepository.Consume()
	if err != nil {
		log.Printf("Не удалось использовать задачи из очереди: %v", err)
	}
	for rpc := range rpcTasks {
		rpcAnswer := models.RPCAnswer{}
		err = json.Unmarshal(rpc, &rpcAnswer)
		if err != nil {
			log.Printf("Не удалось декодировать ответ rpc: %v", err)
			continue
		}
		o.subExpressionRepository.UpdateSubExpressionAgent(ctx, rpcAnswer.IdSubExpression, rpcAnswer.IdAgent)
	}
}

func (o *Orchestrator) RetrySubExpressions(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	for _ = range ticker.C {
		agents, _ := o.agentRepository.GetAgents()
		for _, agent := range agents {
			timeAgent := time.Unix(agent.Heartbeat, 0)
			// Если от агента не поступает ответа в течение retrySubExpressionTimout
			if time.Now().Add(-o.retrySubExpressionTimout).After(timeAgent) {
				agentId, _ := uuid.Parse(agent.Id)
				// получаем все невыполненные subexpression этого агента
				tempExpressions, err := o.subExpressionRepository.GetNotCalculatedSubExpressionsByAgentId(ctx, agentId)
				if err != nil {
					log.Printf("ошибка получения подвыражения по идентификатору агента id %e", err)
					continue
				}
				for _, expr := range tempExpressions {
					oldId := expr.Id
					// удаляем subexpression
					err = o.subExpressionRepository.DeleteSubExpressionById(ctx, expr.Id)
					if err != nil {
						log.Printf("ошибка удаления подвыражений по идентификатору агента id %e", err)
						continue
					}
					// создаем новый
					newExpr, err := o.subExpressionRepository.CreateSubExpression(ctx, expr)
					if err != nil {
						log.Printf("ошибка создания подвыражений %e", err)
						continue
					}
					// меняем у зависимых от удаленного выражения sub_expression на новый
					err = o.subExpressionRepository.ReplaceExpressionsIds(ctx, oldId, newExpr.Id)
					if err != nil {
						log.Printf("ошибка удаления подвыражения агентом id %e", err)
						continue
					}
				}
			}
		}
	}
}
