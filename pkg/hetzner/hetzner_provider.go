package hetzner

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-kit/kit/log/term"
	"github.com/gosuri/uiprogress"
	"github.com/hetznercloud/hcloud-go/hcloud"
	"github.com/xetys/hetzner-kube/pkg/clustermanager"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

type Provider struct {
	client        *hcloud.Client
	context       context.Context
	nodes         []clustermanager.Node
	clusterName   string
	cloudInitFile string
	wait          bool
	token         string
}

// NewHetznerProvider returns an instance of hetzner.Provider
func NewHetznerProvider(clusterName string, client *hcloud.Client, context context.Context, token string) *Provider {

	return &Provider{client: client, context: context, clusterName: clusterName, token: token}
}

// SetCloudInitFile sets cloud init file for node provisioning
func (provider *Provider) SetCloudInitFile(cloudInitFile string) {
	provider.cloudInitFile = cloudInitFile
}

// CreateNodes creates hetzner nodes
func (provider *Provider) CreateNodes(suffix string, template clustermanager.Node, datacenters []string, count int, offset int) ([]clustermanager.Node, error) {
	sshKey, _, err := provider.client.SSHKey.Get(provider.context, template.SSHKeyName)

	if err != nil {
		return nil, err
	}

	serverNameTemplate := fmt.Sprintf("%s-%s-@idx", provider.clusterName, suffix)
	serverOptsTemplate := hcloud.ServerCreateOpts{
		Name: serverNameTemplate,
		ServerType: &hcloud.ServerType{
			Name: template.Type,
		},
		Image: &hcloud.Image{
			Name: "ubuntu-16.04",
		},
	}

	if len(provider.cloudInitFile) > 0 {
		buf, err := ioutil.ReadFile(provider.cloudInitFile)
		if err == nil {
			serverOptsTemplate.UserData = string(buf)
		}

	}

	serverOptsTemplate.SSHKeys = append(serverOptsTemplate.SSHKeys, sshKey)

	datacentersCount := len(datacenters)

	var nodes []clustermanager.Node
	for i := 1; i <= count; i++ {
		var serverOpts hcloud.ServerCreateOpts
		serverOpts = serverOptsTemplate
		nodeNumber := i + offset
		serverOpts.Name = strings.Replace(serverNameTemplate, "@idx", fmt.Sprintf("%.02d", nodeNumber), 1)
		serverOpts.Datacenter = &hcloud.Datacenter{
			Name: datacenters[i%datacentersCount],
		}

		// create
		server, err := provider.runCreateServer(&serverOpts)

		if err != nil {
			return nil, err
		}

		ipAddress := server.Server.PublicNet.IPv4.IP.String()
		log.Printf("Created node '%s' with IP %s", server.Server.Name, ipAddress)

		// render private IP address
		privateIpLastBlock := nodeNumber
		if !template.IsEtcd {
			privateIpLastBlock += 10
			if !template.IsMaster {
				privateIpLastBlock += 10
			}
		}
		privateIpAddress := fmt.Sprintf("10.0.1.%d", privateIpLastBlock)

		node := clustermanager.Node{
			Name:             serverOpts.Name,
			Type:             serverOpts.ServerType.Name,
			IsMaster:         template.IsMaster,
			IsEtcd:           template.IsEtcd,
			IPAddress:        ipAddress,
			PrivateIPAddress: privateIpAddress,
			SSHKeyName:       template.SSHKeyName,
		}
		nodes = append(nodes, node)
		provider.nodes = append(provider.nodes, node)
	}

	return nodes, nil
}

// CreateEtcdNodes creates nodes with type 'etcd'
func (provider *Provider) CreateEtcdNodes(sshKeyName string, masterServerType string, datacenters []string, count int) error {
	template := clustermanager.Node{SSHKeyName: sshKeyName, IsEtcd: true, Type: masterServerType}
	_, err := provider.CreateNodes("etcd", template, datacenters, count, 0)
	return err
}

// CreateMasterNodes creates nodes with type 'master'
func (provider *Provider) CreateMasterNodes(sshKeyName string, masterServerType string, datacenters []string, count int, isEtcd bool) error {
	template := clustermanager.Node{SSHKeyName: sshKeyName, IsMaster: true, Type: masterServerType, IsEtcd: isEtcd}
	_, err := provider.CreateNodes("master", template, datacenters, count, 0)
	return err
}

// CreateWorkerNodes
func (provider *Provider) CreateWorkerNodes(sshKeyName string, workerServerType string, datacenters []string, count int, offset int) ([]clustermanager.Node, error) {
	template := clustermanager.Node{SSHKeyName: sshKeyName, IsMaster: false, Type: workerServerType}
	nodes, err := provider.CreateNodes("worker", template, datacenters, count, offset)
	return nodes, err
}

// GetAllNodes retrieves all nodes
func (provider *Provider) GetAllNodes() []clustermanager.Node {

	return provider.nodes
}

// SetNodes
func (provider *Provider) SetNodes(nodes []clustermanager.Node) {
	provider.nodes = nodes
}

// GetMasterNodes returns master nodes only
func (provider *Provider) GetMasterNodes() []clustermanager.Node {
	nodes := []clustermanager.Node{}
	for _, node := range provider.nodes {
		if node.IsMaster {
			nodes = append(nodes, node)
		}
	}

	return nodes
}

// GetEtcdNodes returns etcd nodes only
func (provider *Provider) GetEtcdNodes() []clustermanager.Node {

	nodes := []clustermanager.Node{}
	for _, node := range provider.nodes {
		if node.IsEtcd {
			nodes = append(nodes, node)
		}
	}

	return nodes
}

// GetWorkerNodes returns worker nodes only
func (provider *Provider) GetWorkerNodes() []clustermanager.Node {
	nodes := []clustermanager.Node{}
	for _, node := range provider.nodes {
		if !node.IsMaster && !node.IsEtcd {
			nodes = append(nodes, node)
		}
	}

	return nodes
}

// GetMasterNode returns the first master node or fail, if no master nodes are found
func (provider *Provider) GetMasterNode() (*clustermanager.Node, error) {
	for _, node := range provider.nodes {
		if node.IsMaster {
			return &node, nil
		}
	}

	return nil, errors.New("no master node found")
}

// GetCluster returns a template for Cluster
func (provider *Provider) GetCluster() clustermanager.Cluster {

	return clustermanager.Cluster{
		Name:  provider.clusterName,
		Nodes: provider.nodes,
	}
}

// GetAdditionalMasterInstallCommands
func (provider *Provider) GetAdditionalMasterInstallCommands() []clustermanager.NodeCommand {

	return []clustermanager.NodeCommand{}
}

// MustWait returns true, if we have to wait after creation for some time
func (provider *Provider) MustWait() bool {
	return provider.wait
}

// Token returns the hcloud token
func (provider *Provider) Token() string {
	return provider.token
}

func (provider *Provider) runCreateServer(opts *hcloud.ServerCreateOpts) (*hcloud.ServerCreateResult, error) {

	log.Printf("creating server '%s'...", opts.Name)
	server, _, err := provider.client.Server.GetByName(provider.context, opts.Name)
	if err != nil {
		return nil, err
	}
	if server == nil {
		result, _, err := provider.client.Server.Create(provider.context, *opts)
		if err != nil {
			if err.(hcloud.Error).Code == "uniqueness_error" {
				server, _, err := provider.client.Server.Get(provider.context, opts.Name)

				if err != nil {
					return nil, err
				}

				return &hcloud.ServerCreateResult{Server: server}, nil
			}

			return nil, err
		}

		if err := provider.actionProgress(result.Action); err != nil {
			return nil, err
		}

		provider.wait = true

		return &result, nil
	} else {
		log.Printf("loading server '%s'...", opts.Name)
		return &hcloud.ServerCreateResult{Server: server}, nil
	}
}

func (provider *Provider) actionProgress(action *hcloud.Action) error {
	errCh, progressCh := waitAction(provider.context, provider.client, action)

	if term.IsTerminal(os.Stdout) {
		progress := uiprogress.New()

		progress.Start()
		bar := progress.AddBar(100).AppendCompleted().PrependElapsed()
		bar.Width = 40
		bar.Empty = ' '

		for {
			select {
			case err := <-errCh:
				if err == nil {
					bar.Set(100)
				}
				progress.Stop()
				return err
			case p := <-progressCh:
				bar.Set(p)
			}
		}
	} else {
		return <-errCh
	}
}
