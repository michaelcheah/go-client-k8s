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
	First *callback
	Second *callback
	Third *callback
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
	go func() {
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

func NewLogger(clientset seldonclientset.Clientset) *Informer {
	informerFactory := seldonfactory.NewSharedInformerFactory(&clientset, 30*time.Second)
	deploymentInformer := informerFactory.Machinelearning().V1().SeldonDeployments().Informer()

	informer := &Informer{
		factory:         informerFactory,
		stopChan:        make(chan struct{}),
		forLoopStopChan: make(chan bool),
		diffMatchParam:  diffmatchpatch.New(),
		lastDeploy:      machinelearningv1.SeldonDeployment{},
		First: NewCallback(),
		Second: NewCallback(),
		Third: NewCallback(),

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
	fmt.Println("we are sending the for loop stop")
	i.forLoopStopChan <- true
	i.stopChan <- struct{}{}
	fmt.Println("we have sent the for loop stop")
}

func (i *Informer) WaitTillEnd() {
	defer close(i.stopChan)
	<-i.stopChan
	fmt.Println("we finished")
}

func (i *Informer) add(obj interface{}) {
	deploy := obj.(*machinelearningv1.SeldonDeployment)
	fmt.Printf("==> Deployment ADDED: \n")
	i.printDiffFromLastEvent(deploy)
}

func (i *Informer) delete(obj interface{}) {
	deploy := obj.(*machinelearningv1.SeldonDeployment)
	fmt.Printf("==> Deployment DELETED: \n")
	i.printDiffFromLastEvent(deploy)
	i.Third.triggerChan <- true
	fmt.Println("resources have been deleteeeeeed")
}

func (i *Informer) update(oldObj, newObj interface{}) {
	newDeploy := newObj.(*machinelearningv1.SeldonDeployment)
	oldDeploy := oldObj.(*machinelearningv1.SeldonDeployment)
	if newDeploy.ResourceVersion == oldDeploy.ResourceVersion {
		// only update when new is different from old.
		fmt.Printf("NOTHING HAS CHANGED \n")
		return
	}
	fmt.Printf("==> Deployment CHANGED: \n")
	i.printDiffFromLastEvent(newDeploy)

	if createResourcesAreAvailable(newDeploy) && i.First.running {
		fmt.Println("resources have been made available")
		i.First.triggerChan <- true
	}

	if replicasHaveBeenScaled(newDeploy) && i.Second.running {
		fmt.Println("replicas have been scaleddddddddddddddddddddddddddddddddddd")
		i.Second.triggerChan <- true
	}
}

func createResourcesAreAvailable(deploy *machinelearningv1.SeldonDeployment) bool {
	if deploy.Status.State != machinelearningv1.StatusStateAvailable {
		fmt.Println("NOT AVAILABLE")
		return false
	}

	if len(deploy.Status.ServiceStatus) < 2 {
		fmt.Println("NOT YET 2, GOT %d", len(deploy.Status.ServiceStatus))
		return false
	}
	return true
}

func replicasHaveBeenScaled(deploy *machinelearningv1.SeldonDeployment) bool {
	if deploy.Spec.Replicas != nil {
		fmt.Printf("CHECKING THIS NOWWWWWW, %d\n", *deploy.Spec.Replicas)
		if *deploy.Spec.Replicas == int32(2) {
			return true
		}
	} else {
		fmt.Println("STILL NILLLLLLLLLLLL")
	}
	return false
}

func resourceDeleted(deploy *machinelearningv1.SeldonDeployment) bool {
	if deploy.Status.Replicas == 0 {
		return true
	}
	return false
}


func (i *Informer) printDiffFromLastEvent(newDeploy *machinelearningv1.SeldonDeployment) {
	//diffs := i.diffMatchParam.DiffMain(prettyPrint(i.lastDeploy.Status), prettyPrint(newDeploy.Status), false)
	//fmt.Println(i.diffMatchParam.DiffPrettyText(diffs))
	//i.lastDeploy = *newDeploy
	diffs := i.diffMatchParam.DiffMain(prettyPrint(i.lastDeploy), prettyPrint(newDeploy), false)
	fmt.Println(i.diffMatchParam.DiffPrettyText(diffs))
	i.lastDeploy = *newDeploy

}

func prettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}

