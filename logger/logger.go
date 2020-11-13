package logger

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	machinelearningv1 "github.com/seldonio/seldon-core/operator/apis/machinelearning.seldon.io/v1"
	seldonclientset "github.com/seldonio/seldon-core/operator/client/machinelearning.seldon.io/v1/clientset/versioned"
	"log"
	"sync"
	"time"

	//seldondeployment "github.com/seldonio/seldon-core/operator/client/machinelearning.seldon.io/v1/clientset/versioned/typed/machinelearning.seldon.io/v1"
	//seldoninformer "github.com/seldonio/seldon-core/operator/client/machinelearning.seldon.io/v1alpha3/informers/externalversions/machinelearning.seldon.io/v1alpha3"
	seldonfactory "github.com/seldonio/seldon-core/operator/client/machinelearning.seldon.io/v1/informers/externalversions"
	"github.com/sergi/go-diff/diffmatchpatch"
	"k8s.io/client-go/tools/cache"
)

type Informer struct {
	factory         seldonfactory.SharedInformerFactory
	stopChan        chan struct{}
	forLoopStopChan chan bool
	diffMatchParam  *diffmatchpatch.DiffMatchPatch
	lastDeploy      machinelearningv1.SeldonDeployment
	First           *callback
	Second          *callback
	Third           *callback
	debug           bool
}

type callback struct {
	triggerChan chan bool
	funcs       []func() error
	mutex       sync.Mutex
	stopChan    chan bool
	running     bool
}

func (c *callback) applyFuncs() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, function := range c.funcs {
		err := function()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *callback) RegisterFunc(f func() error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.funcs = append(c.funcs, f)
}

func (c *callback) Stop() {
	c.stopChan <- true
}

func NewCallback() *callback {
	c := &callback{
		triggerChan: make(chan bool),
		mutex:       sync.Mutex{},
		stopChan:    make(chan bool),
	}
	go func() { // TODO: This is unnecessarily complicated. Savagely refactor this
		c.running = true
		select {
		case <-c.triggerChan:
			err := c.applyFuncs()
			if err != nil {
				log.Printf("%+v\n", errors.WithStack(err))
			}
		case <-c.stopChan:
		}
		c.running = false
	}()
	return c
}

func NewLogger(clientset seldonclientset.Clientset, debugFlag bool) *Informer {
	informerFactory := seldonfactory.NewSharedInformerFactory(&clientset, 30*time.Second)
	deploymentInformer := informerFactory.Machinelearning().V1().SeldonDeployments().Informer()

	informer := &Informer{
		factory:         informerFactory,
		stopChan:        make(chan struct{}),
		forLoopStopChan: make(chan bool),
		diffMatchParam:  diffmatchpatch.New(),
		lastDeploy:      machinelearningv1.SeldonDeployment{},
		First:           NewCallback(), // TODO: This is hacky. Refactor this
		Second:          NewCallback(),
		Third:           NewCallback(),
		debug:           debugFlag,
	}

	deploymentInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    informer.add,
		DeleteFunc: informer.delete,
		UpdateFunc: informer.update,
	})

	return informer
}

func (i *Informer) Run() {
	i.factory.Start(i.stopChan)
	for {
		select {
		case <-time.Tick(1 * time.Second):
		case <-i.forLoopStopChan:
			fmt.Println("exiting loop")
			return
		}
	}
}

func (i *Informer) Stop() {
	i.forLoopStopChan <- true
	i.stopChan <- struct{}{}
}

func (i *Informer) WaitTillEnd() {
	defer close(i.stopChan)
	<-i.stopChan
	fmt.Println("Logger has finished")
}

func (i *Informer) add(obj interface{}) {
	i.debugLog("Deployment has been added")
	deploy := obj.(*machinelearningv1.SeldonDeployment)
	i.printDiffFromLastEvent(deploy)
}

func (i *Informer) delete(obj interface{}) {
	i.debugLog("Deployment has been deleted")
	deploy := obj.(*machinelearningv1.SeldonDeployment)
	i.printDiffFromLastEvent(deploy)
	i.Third.triggerChan <- true
}

func (i *Informer) update(oldObj, newObj interface{}) {
	i.debugLog("Deployment has been updated")
	newDeploy := newObj.(*machinelearningv1.SeldonDeployment)
	oldDeploy := oldObj.(*machinelearningv1.SeldonDeployment)
	if newDeploy.ResourceVersion == oldDeploy.ResourceVersion {
		// only update when new is different from old.
		return
	}
	i.printDiffFromLastEvent(newDeploy)

	if createResourcesAreAvailable(newDeploy) && i.First.running {
		i.debugLog("Resources are now available")
		i.First.triggerChan <- true
	}

	if replicasHaveBeenScaled(newDeploy) && i.Second.running {
		i.debugLog("Replicas have been scaled")
		i.Second.triggerChan <- true
	}
}

// TODO: These boolean functions are weird. I want the deployer to control these definitions
func createResourcesAreAvailable(deploy *machinelearningv1.SeldonDeployment) bool {
	if deploy.Status.State != machinelearningv1.StatusStateAvailable {
		return false
	}

	return true
}

func replicasHaveBeenScaled(deploy *machinelearningv1.SeldonDeployment) bool {
	if deploy.Spec.Replicas != nil {
		if *deploy.Spec.Replicas == int32(2) { //TODO: Change this to be configurable
			return true
		}
	}
	return false
}

// TODO: Replace this with a proper logger library
// TODO: Attach a name to the logger and have this display it so we can tell where the log came from
func (i *Informer) debugLog(s string) {
	if i.debug {
		log.Println(s)
	}
}

func (i *Informer) printDiffFromLastEvent(newDeploy *machinelearningv1.SeldonDeployment) {
	if i.debug {
		diffs := i.diffMatchParam.DiffMain(prettyPrint(i.lastDeploy), prettyPrint(newDeploy), false)
		fmt.Println(i.diffMatchParam.DiffPrettyText(diffs))
		i.lastDeploy = *newDeploy
	}
}

func prettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}
