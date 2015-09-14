/*
** Copyright [2013-2015] [Megam Systems]
**
** Licensed under the Apache License, Version 2.0 (the "License");
** you may not use this file except in compliance with the License.
** You may obtain a copy of the License at
**
** http://www.apache.org/licenses/LICENSE-2.0
**
** Unless required by applicable law or agreed to in writing, software
** distributed under the License is distributed on an "AS IS" BASIS,
** WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
** See the License for the specific language governing permissions and
** limitations under the License.
 */

package one

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/megamsys/megamd/router"
	"github.com/megamsys/opennebula-go/api"
	"github.com/megamsys/megamd/provision"
)


var mainOneProvisioner *oneProvisioner

func init() {
	mainOneProvisioner = &oneProvisioner{}
	provision.Register("one", mainOneProvisioner)
}

type oneProvisioner struct {
	client *api.RPCClient
}


func (p *oneProvisioner) Initialize(m map[string]string) error {
	return p.initOneCluster(m)
}

func (p *oneProvisioner) initOneCluster(m map[string]string) error {
	client, err := api.NewRPCClient(m[provision.ONE_ENDPOINT], m[provision.ONE_USERID], m[provision.ONE_PASSWORD])
	p.client = client
	return err
}

func getRouterForBox(box provision.Box) (router.Router, error) {
	routerName, err := box.GetRouter()
	if err != nil {
		return nil, err
	}
	return router.Get(routerName)
}

func (p *oneProvisioner) StartupMessage() (string, error) {
	out := "One provisioner reports the following:\n"
	out += fmt.Sprintf("    One xmlrpc initiated: %#v\n", p.client)
	return out, nil
}

func (p *oneProvisioner) GitDeploy(box Box, version string, w io.Writer) (string, error) {
	return nil, nil
}

func (p *oneProvisioner) ImageDeploy(box Box, imageId string, w io.Writer) (string, error) {
	isValid, err := isValidBoxImage(box.GetName(), imageId)
	if err != nil {
		return "", err
	}
	if !isValid {
		return "", fmt.Errorf("invalid image for box %s: %s", box.GetName(), imageId)
	}
	return imageId, p.deployPipeline(box, imageId, w)
}

//start by validating the image.
//1. &updateStatus in Riak - Deploying..
//2. &create an inmemory machine type from a Box.
//3. &updateStatus in Riak - Creating..
//4. &followLogs by posting it in the queue.
func (p *oneProvisioner) deployPipeline(box Box, imageId string, w io.Writer) (string, error) {
	fmt.Fprintf(w, "\n---- Create %s box %s %s ----\n", box.GetName(), imageId)
	actions := []*action.Action{
		&updateStatusInRiak,
		&createMachine,
		&updateStatusInRiak,
		&followLogs,
	}
	pipeline := action.NewPipeline(actions...)

	args := runMachineActionsArgs{
		box:             box,
		imageID:         imageId,
		writer:          w,
		isDeploy:        true,
		deployingStatus: StatusDeploying,
		provisioner:     p,
	}

	err := pipeline.Execute(args)
	if err != nil {
		log.Errorf("error on execute deploy pipeline for box %s - %s", box.GetName(), err)
		return "", err
	}
	return imageId, nil
}

func (p *oneProvisioner) Destroy(box provision.Box, w io.Writer) error {
	fmt.Fprintf(w, "\n---- Removing %s ----\n", box.GetName())
	args := nukeUnitsPipelineArgs{
		app:         box,
		toRemove:    boxs,
		writer:      w,
		provisioner: p,
		boxDestroy:  true,
	}

	actions := []*action.Action{
		&followLogs,
		&updateStatusInRiak,
		&removeOldMachine,
		&removeBoxesInRiak,
		&removeCartonsInRiak,
		&provisionUnbindOldUnits,
		&removeOldRoutes,
	}

	pipeline := action.NewPipeline(actions...)

	err = pipeline.Execute(args)
	if err != nil {
		return err
	}

	return nil
}

func (p *oneProvisioner) Restart(box Box, w io.Writer) error {
	return nil
}

func (p *oneProvisioner) Start(box Box) error {
	return nil
}

func (p *oneProvisioner) Stop(box Box) error {
	return nil
}

func (*oneProvisioner) Addr(box provision.Box) (string, error) {
	r, err := getRouterForApp(box)
	if err != nil {
		log.Errorf("Failed to get router: %s", err)
		return "", err
	}
	addr, err := r.Addr(box.GetName())
	if err != nil {
		log.Errorf("Failed to obtain box %s address: %s", box.GetName(), err)
		return "", err
	}
	return addr, nil
}

func (p *oneProvisioner) SetBoxStatus(box provision.Box, status provision.Status) error {
	fmt.Fprintf(w, "\n---- status %s box %s %s ----\n", box.GetName(), status.String())
	actions := []*action.Action{
		&updateStatusInRiak,
	}
	pipeline := action.NewPipeline(actions...)

	args := runMachineActionsArgs{
		box:             box,
		writer:          w,
		deployingStatus: status,
		provisioner:     p,
	}

	err := pipeline.Execute(args)
	if err != nil {
		log.Errorf("error on execute status pipeline for box %s - %s", box.GetName(), err)
		return err
	}
	return nil
}

func (p *oneProvisioner) ExecuteCommandOnce(stdout, stderr io.Writer, box provision.Box, cmd string, args ...string) error {
	//boxs, err := p.listRunnableMachinesByBox(box.GetName())
	machs, err := []Machine{}

	if err != nil {
		return err
	}
	if len(boxs) == 0 {
		return provision.ErrEmptyBox
	}
	mach := machs[0]
	return mach.Exec(p, stdout, stderr, cmd, args...)
}

func (p *oneProvisioner) SetCName(box provision.Box, cname string) error {
	r, err := getRouterForBox(box)
	if err != nil {
		return err
	}
	return r.SetCName(cname, box.GetName())
}

func (p *oneProvisioner) UnsetCName(box provision.Box, cname string) error {
	r, err := getRouterForBox(box)
	if err != nil {
		return err
	}
	return r.UnsetCName(cname, box.GetName())
}

// PlatformAdd build and push a new template into one
func (p *oneProvisioner) PlatformAdd(name string, args map[string]string, w io.Writer) error {
	return nil
}

func (p *oneProvisioner) PlatformUpdate(name string, args map[string]string, w io.Writer) error {
	return p.PlatformAdd(name, args, w)
}

func (p *oneProvisioner) PlatformRemove(name string) error {
	return nil
}

func (p *oneProvisioner) MetricEnvs(cart carton.Carton) map[string]string {
	envMap := map[string]string{}
	//gadvConf, err := gadvisor.LoadConfig()
	//if err != nil {
	//	return envMap
	//}
	envs, err := []string{}, nil //gadvConf.MetrisList

	if err != nil {
		return envMap
	}
	for _, env := range envs {
		if strings.HasPrefix(env, "METRICS_") {
			slice := strings.SplitN(env, "=", 2)
			envMap[slice[0]] = slice[1]
		}
	}
	return envMap
}
