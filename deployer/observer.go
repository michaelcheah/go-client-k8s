package deployer

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	machinelearningv1 "github.com/seldonio/seldon-core/operator/apis/machinelearning.seldon.io/v1"
	seldonclientset "github.com/seldonio/seldon-core/operator/client/machinelearning.seldon.io/v1/clientset/versioned"
	seldonfactory "github.com/seldonio/seldon-core/operator/client/machinelearning.seldon.io/v1/informers/externalversions"
	"github.com/sergi/go-diff/diffmatchpatch"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/cache"
	"time"
)

// TODO: These seem too similar to watch.EventType
type EventType string

const (
	Added   EventType = "ADDED"
	Updated EventType = "UPDATED"
	Deleted EventType = "DELETED"
)

// TODO: Consider if this is just a reimplementation of the watch.Event type?
type Event struct {
	Deployment *machinelearningv1.SeldonDeployment
	Type       EventType
}

type ObserverV2 struct { // TODO: Rename this to Observer. Weird IDE bug
	factory          seldonfactory.SharedInformerFactory
	NotifyFunc       func(Event) error
	ErrorFunc        func()
	stopInformerChan chan struct{}
	stopObserverChan chan bool
	notifyChan       chan Event
	diffMatchParam   *diffmatchpatch.DiffMatchPatch
	lastDeploy       machinelearningv1.SeldonDeployment
	stopContext      context.Context
	cancelFunc       func()
}

func NewObserver(clientset *seldonclientset.Clientset) *ObserverV2 {
	informerFactory := seldonfactory.NewSharedInformerFactory(clientset, 30*time.Second)
	deploymentInformer := informerFactory.Machinelearning().V1().SeldonDeployments().Informer()

	stopContext, cancelFunc := context.WithCancel(context.Background())

	observer := &ObserverV2{
		factory:          informerFactory,
		stopInformerChan: make(chan struct{}),
		stopObserverChan: make(chan bool),
		notifyChan:       make(chan Event),
		diffMatchParam:   diffmatchpatch.New(),
		lastDeploy:       machinelearningv1.SeldonDeployment{},
		stopContext:      stopContext,
		cancelFunc:       cancelFunc,
	}

	deploymentInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    observer.add,
		DeleteFunc: observer.delete,
		UpdateFunc: observer.update,
	})

	return observer
}

func (o *ObserverV2) WaitTillContextIsCancelled(ctx context.Context) {
	select {
	case <-ctx.Done():
		log.Info(EventLog("Context has been cancelled successfully"))
	}
	o.cancelFunc()
}

// Main event loop. To be called in a go routine
func (o *ObserverV2) Run() {
	defer close(o.stopInformerChan) // TODO: According to documentation, the informer is stopped when the stopchan is closed. Verify this.
	o.factory.Start(o.stopInformerChan)
	err := o.notifyLoop()
	if err != nil {
		log.Error(errors.Wrap(err, "exited notify loop"))
		if o.ErrorFunc != nil {
			log.Info("calling notify error handler function...")
			o.ErrorFunc()
		}
	}
}

// This is our main event loop.
// The notifyChan is constantly read.
func (o ObserverV2) notifyLoop() error {
	for {
		select {
		case event := <-o.notifyChan:
			o.printDiffFromLastEvent(event.Deployment)
			err := o.NotifyFunc(event)
			if err != nil {
				return errors.Wrapf(err, "NotifyFunc of %s event failed. Exiting notify loop", event.Type)
			}
		case <-o.stopContext.Done():
			log.Errorf("context cancelled for notify loop")
			return fmt.Errorf("context cancelled for notify loop")
		}
	}
}

func (o *ObserverV2) sendToNotifyLoop(event Event) {
	log.Infof(DescriptionLog("[KUBERNETES EVENT] [Deployment %s] %s", event.Type, event.Deployment.Status.Description))
	select {
	case o.notifyChan <- event:
	case <-o.stopContext.Done():
	}
}

func (o *ObserverV2) add(obj interface{}) {
	deploy := obj.(*machinelearningv1.SeldonDeployment)
	o.sendToNotifyLoop(Event{deploy, Added})
}

func (o *ObserverV2) delete(obj interface{}) {
	deploy := obj.(*machinelearningv1.SeldonDeployment)
	o.sendToNotifyLoop(Event{deploy, Deleted})
}

func (o *ObserverV2) update(oldObj, newObj interface{}) {
	newDeploy := newObj.(*machinelearningv1.SeldonDeployment)
	oldDeploy := oldObj.(*machinelearningv1.SeldonDeployment)
	if newDeploy.ResourceVersion == oldDeploy.ResourceVersion {
		// only update when new is different from old.
		log.Info("Resource version is the same")
		return
	}
	o.sendToNotifyLoop(Event{newDeploy, Updated})
}

func (o *ObserverV2) printDiffFromLastEvent(newDeploy *machinelearningv1.SeldonDeployment) {
	diffs := o.diffMatchParam.DiffMain(prettyPrint(o.lastDeploy), prettyPrint(newDeploy), false)
	log.Debug(o.diffMatchParam.DiffPrettyText(diffs))
	o.lastDeploy = *newDeploy

}

func prettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}
