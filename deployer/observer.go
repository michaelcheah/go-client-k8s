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
	"k8s.io/client-go/tools/cache"
	"log"
	"time"
)

// TODO: These seem too similar to watch.EventType
type EventType string

const (
	ADDED   EventType = "added"
	UPDATED EventType = "updated"
	DELETED EventType = "deleted"
)

// TODO: Consider if this is just a reimplementation of the watch.Event type?
type Event struct {
	Deployment *machinelearningv1.SeldonDeployment
	Type       EventType
}

type ObserverV2 struct { // TODO: Rename this to Observer. Weird IDE bug
	factory          seldonfactory.SharedInformerFactory
	NotifyFunc       func(Event) error
	ErrorFunc        func(error)
	stopInformerChan chan struct{}
	stopObserverChan chan bool
	notifyChan       chan Event
	debug            bool
	diffMatchParam   *diffmatchpatch.DiffMatchPatch
	lastDeploy       machinelearningv1.SeldonDeployment
}

func NewObserver(clientset seldonclientset.Clientset, debugFlag bool) *ObserverV2 {
	informerFactory := seldonfactory.NewSharedInformerFactory(&clientset, 30*time.Second)
	deploymentInformer := informerFactory.Machinelearning().V1().SeldonDeployments().Informer()

	observer := &ObserverV2{
		factory:          informerFactory,
		stopInformerChan: make(chan struct{}),
		diffMatchParam:   diffmatchpatch.New(),
		lastDeploy:       machinelearningv1.SeldonDeployment{},
		notifyChan:       make(chan Event),
		debug:            debugFlag,
	}

	deploymentInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    observer.add,
		DeleteFunc: observer.delete,
		UpdateFunc: observer.update,
	})

	return observer
}

func (o *ObserverV2) WaitTillEnd(ctx context.Context) {
	select {
	case <-o.stopObserverChan: // TODO: Figure out where this is actually called
		log.Println("Stopped successfully") // TODO: Use the custom logger
	case <-ctx.Done():
		log.Println("Context has been cancelled")
	}
}

// Main event loop. To be called in a go routine
func (o *ObserverV2) Run(ctx context.Context) {
	defer close(o.stopInformerChan) // TODO: According to documentation, the informer is stopped when the stopchan is closed. Verify this.
	err := o.notifyLoop(ctx)
	if err != nil {
		o.ErrorFunc(err)
	}
	o.stopObserverIfWaiting()
}

// This is our main event loop.
// The notifyChan is constantly read.
func (o ObserverV2) notifyLoop(ctx context.Context) error {
	for {
		select {
		case event := <-o.notifyChan:
			o.debugLog(fmt.Sprintf("==> %s event has been received\n", event.Type))
			o.printDiffFromLastEvent(event.Deployment)
			err := o.NotifyFunc(event)
			if err != nil {
				return errors.Wrapf(err, "NotifyFunc of %s event failed. Exiting NotifyFunc loop", event.Type)
			}
		case <-ctx.Done():
			return fmt.Errorf("context cancelled for NotifyFunc loop")
		}
	}
}

func (o *ObserverV2) sendToNotifyLoop(event Event) {
	select {
	case o.notifyChan <- event:
	default:
		o.stopObserverIfWaiting()
		log.Fatalf("NotifyFunc loop is not running. This should be impossible")
	}
}

// Non-blocking send in case user did not call WaitTillEnd
func (o *ObserverV2) stopObserverIfWaiting() {
	select {
	case o.stopObserverChan <- true:
	default:
	}
}

func (o *ObserverV2) add(obj interface{}) {
	deploy := obj.(*machinelearningv1.SeldonDeployment)
	o.sendToNotifyLoop(Event{deploy, ADDED})
}

func (o *ObserverV2) delete(obj interface{}) {
	deploy := obj.(*machinelearningv1.SeldonDeployment)
	o.sendToNotifyLoop(Event{deploy, DELETED})
}

func (o *ObserverV2) update(oldObj, newObj interface{}) {
	newDeploy := newObj.(*machinelearningv1.SeldonDeployment)
	oldDeploy := oldObj.(*machinelearningv1.SeldonDeployment)
	if newDeploy.ResourceVersion == oldDeploy.ResourceVersion {
		// only update when new is different from old.
		o.debugLog("Resource version is the same")
		return
	}
	o.sendToNotifyLoop(Event{newDeploy, UPDATED})
}

// TODO: Replace this with a proper logger library
// TODO: Attach a name to the logger and have this display it so we can tell where the log came from
func (o *ObserverV2) debugLog(s string) {
	if o.debug {
		log.Println(s)
	}
}

func (o *ObserverV2) printDiffFromLastEvent(newDeploy *machinelearningv1.SeldonDeployment) {
	if o.debug {
		diffs := o.diffMatchParam.DiffMain(prettyPrint(o.lastDeploy), prettyPrint(newDeploy), false)
		fmt.Println(o.diffMatchParam.DiffPrettyText(diffs))
		o.lastDeploy = *newDeploy
	}
}

func prettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}
