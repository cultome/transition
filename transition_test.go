package transition_test

import (
	"errors"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/cultome/transition"
)

type Order struct {
	transition.Transition

	Address string
}

func createOrder() *Order {
	order := Order{}
	db.Create(&order)

	return &order
}

func acquireConnection() *gorm.DB {
	connString := "postgresql://username:password@localhost:5432/transition"
	db, err := gorm.Open(postgres.Open(connString), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	return db
}

var db = acquireConnection()

func init() {
	for _, model := range []interface{}{&Order{}, &transition.StateChangeLog{}} {
		db.Migrator().DropTable(model)
	}

	for _, model := range []interface{}{&Order{}, &transition.StateChangeLog{}} {
		db.AutoMigrate(model)
	}
}

func getStateMachine() *transition.StateMachine {
	var orderStateMachine = transition.New(&Order{})

	orderStateMachine.Initial("draft")
	orderStateMachine.State("checkout")
	orderStateMachine.State("paid")
	orderStateMachine.State("processed")
	orderStateMachine.State("delivered")
	orderStateMachine.State("cancelled")
	orderStateMachine.State("paid_cancelled")

	orderStateMachine.Event("checkout").To("checkout").From("draft")
	orderStateMachine.Event("pay").To("paid").From("checkout")

	return orderStateMachine
}

func CreateOrderAndExecuteTransition(transition *transition.StateMachine, event string, order *Order) error {
	if err := db.Save(order).Error; err != nil {
		return err
	}

	if err := transition.Trigger(event, order, db); err != nil {
		return err
	}
	return nil
}

func TestStateTransition(t *testing.T) {
	order := createOrder()

	if err := getStateMachine().Trigger("checkout", order, db); err != nil {
		t.Errorf("should not raise any error when trigger event checkout")
	}

	if order.GetState() != "checkout" {
		t.Errorf("state doesn't changed to checkout")
	}

	var stateChangeLogs = transition.GetStateChangeLogs(order, db)
	if len(stateChangeLogs) != 1 {
		t.Errorf("should get one state change log with GetStateChangeLogs")
	} else {
		var stateChangeLog = stateChangeLogs[0]

		if stateChangeLog.From != "draft" {
			t.Errorf("state from not set")
		}

		if stateChangeLog.To != "checkout" {
			t.Errorf("state to not set")
		}
	}
}

func TestStateTransitionPreload(t *testing.T) {
	order := createOrder()

	if err := getStateMachine().Trigger("checkout", order, db); err != nil {
		t.Errorf("should not raise any error when trigger event checkout")
	}

	if order.GetState() != "checkout" {
		t.Errorf("state doesn't changed to checkout")
	}

	dbOrder := Order{}
	db.Preload("StateChangeLogs").First(&dbOrder)

	if len(dbOrder.StateChangeLogs) < 1 {
		t.Errorf("state changes are not being loaded")
	}

	for _, stateChangeLog := range dbOrder.StateChangeLogs {
		if stateChangeLog.From != "draft" {
			t.Errorf("state from not set")
		}

		if stateChangeLog.To != "checkout" {
			t.Errorf("state to not set")
		}
	}
}

func TestGetLastStateChange(t *testing.T) {
	order := createOrder()

	if err := getStateMachine().Trigger("checkout", order, db, "checkout note"); err != nil {
		t.Errorf("should not raise any error when trigger event checkout")
	}

	if err := getStateMachine().Trigger("pay", order, db, "pay note"); err != nil {
		t.Errorf("should not raise any error when trigger event checkout")
	}

	if order.GetState() != "paid" {
		t.Errorf("state doesn't changed to paid")
	}

	var lastStateChange = transition.GetLastStateChange(order, db)
	if lastStateChange.To != "paid" {
		t.Errorf("state to not set")
	} else {
		if lastStateChange.From != "checkout" {
			t.Errorf("state from not set")
		}

		if lastStateChange.Note != "pay note" {
			t.Errorf("state note not set")
		}
	}
}

func TestMultipleTransitionWithOneEvent(t *testing.T) {
	orderStateMachine := getStateMachine()
	cancellEvent := orderStateMachine.Event("cancel")
	cancellEvent.To("cancelled").From("draft", "checkout")
	cancellEvent.To("paid_cancelled").From("paid", "processed")

	unpaidOrder1 := &Order{}
	if err := orderStateMachine.Trigger("cancel", unpaidOrder1, db); err != nil {
		t.Errorf("should not raise any error when trigger event cancel")
	}

	if unpaidOrder1.State != "cancelled" {
		t.Errorf("order status doesn't transitioned correctly")
	}

	unpaidOrder2 := &Order{}
	unpaidOrder2.State = "draft"
	if err := orderStateMachine.Trigger("cancel", unpaidOrder2, db); err != nil {
		t.Errorf("should not raise any error when trigger event cancel")
	}

	if unpaidOrder2.State != "cancelled" {
		t.Errorf("order status doesn't transitioned correctly")
	}

	paidOrder := createOrder()
	paidOrder.State = "paid"
	if err := orderStateMachine.Trigger("cancel", paidOrder, db); err != nil {
		t.Errorf("should not raise any error when trigger event cancel")
	}

	if paidOrder.State != "paid_cancelled" {
		t.Errorf("order status doesn't transitioned correctly")
	}
}

func TestStateCallbacks(t *testing.T) {
	orderStateMachine := getStateMachine()
	order := createOrder()

	address1 := "I'm an address should be set when enter checkout"
	address2 := "I'm an address should be set when exit checkout"
	orderStateMachine.State("checkout").Enter(func(order interface{}, db *gorm.DB) error {
		order.(*Order).Address = address1
		return nil
	}).Exit(func(order interface{}, db *gorm.DB) error {
		order.(*Order).Address = address2
		return nil
	})

	if err := orderStateMachine.Trigger("checkout", order, db); err != nil {
		t.Errorf("should not raise any error when trigger event checkout")
	}

	if order.Address != address1 {
		t.Errorf("enter callback not triggered")
	}

	if err := orderStateMachine.Trigger("pay", order, db); err != nil {
		t.Errorf("should not raise any error when trigger event pay")
	}

	if order.Address != address2 {
		t.Errorf("exit callback not triggered")
	}
}

func TestEventCallbacks(t *testing.T) {
	var (
		order                 = &Order{}
		orderStateMachine     = getStateMachine()
		prevState, afterState string
	)

	orderStateMachine.Event("checkout").To("checkout").From("draft").Before(func(order interface{}, db *gorm.DB) error {
		prevState = order.(*Order).State
		return nil
	}).After(func(order interface{}, db *gorm.DB) error {
		afterState = order.(*Order).State
		return nil
	})

	order.State = "draft"
	if err := orderStateMachine.Trigger("checkout", order, nil); err != nil {
		t.Errorf("should not raise any error when trigger event checkout")
	}

	if prevState != "draft" {
		t.Errorf("Before callback triggered after state change")
	}

	if afterState != "checkout" {
		t.Errorf("After callback triggered after state change")
	}
}

func TestTransitionOnEnterCallbackError(t *testing.T) {
	var (
		order             = &Order{}
		orderStateMachine = getStateMachine()
	)

	orderStateMachine.State("checkout").Enter(func(order interface{}, db *gorm.DB) (err error) {
		return errors.New("intentional error")
	})

	if err := orderStateMachine.Trigger("checkout", order, nil); err == nil {
		t.Errorf("should raise an intentional error")
	}

	if order.State != "draft" {
		t.Errorf("state transitioned on Enter callback error")
	}
}

func TestTransitionOnExitCallbackError(t *testing.T) {
	var (
		order             = &Order{}
		orderStateMachine = getStateMachine()
	)

	orderStateMachine.State("checkout").Exit(func(order interface{}, db *gorm.DB) (err error) {
		return errors.New("intentional error")
	})

	if err := orderStateMachine.Trigger("checkout", order, nil); err != nil {
		t.Errorf("should not raise error when checkout")
	}

	if err := orderStateMachine.Trigger("pay", order, nil); err == nil {
		t.Errorf("should raise an intentional error")
	}

	if order.State != "checkout" {
		t.Errorf("state transitioned on Enter callback error")
	}
}

func TestEventOnBeforeCallbackError(t *testing.T) {
	var (
		order             = &Order{}
		orderStateMachine = getStateMachine()
	)

	orderStateMachine.Event("checkout").To("checkout").From("draft").Before(func(order interface{}, db *gorm.DB) error {
		return errors.New("intentional error")
	})

	if err := orderStateMachine.Trigger("checkout", order, nil); err == nil {
		t.Errorf("should raise an intentional error")
	}

	if order.State != "draft" {
		t.Errorf("state transitioned on Enter callback error")
	}
}

func TestEventOnAfterCallbackError(t *testing.T) {
	var (
		order             = &Order{}
		orderStateMachine = getStateMachine()
	)

	orderStateMachine.Event("checkout").To("checkout").From("draft").After(func(order interface{}, db *gorm.DB) error {
		return errors.New("intentional error")
	})

	if err := orderStateMachine.Trigger("checkout", order, nil); err == nil {
		t.Errorf("should raise an intentional error")
	}

	if order.State != "draft" {
		t.Errorf("state transitioned on Enter callback error")
	}
}
