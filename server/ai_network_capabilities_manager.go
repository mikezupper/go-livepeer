package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/glog"
	"github.com/livepeer/go-livepeer/common"
	"github.com/livepeer/go-livepeer/core"
)

// OrchestratorPipeline represents the pipeline information for each model under an orchestrator.
type OrchestratorPipeline struct {
	Warm bool `json:"Warm"`
}

// OrchestratorInfo represents the information for each orchestrator, containing a map of pipelines.
type OrchestratorInfo struct {
	Pipelines map[string]map[string]OrchestratorPipeline `json:"Pipelines"`
}

// SupportedPipelineStats represents the cold and warm statistics for each supported model.
type SupportedPipelineStats struct {
	Cold int `json:"Cold"`
	Warm int `json:"Warm"`
}

// NetworkCapabiltiesManager represents the full structure, with flexible key names.
type NetworkCapabiltiesManager struct {
	Orchestrators      map[string]OrchestratorInfo                  `json:"orchestrators"`
	SupportedPipelines map[string]map[string]SupportedPipelineStats `json:"supported_pipelines"`
}

// NewJSONData creates and initializes a new NetworkCapabiltiesManager structure.
func NewNetworkCapabilitiesManager() *NetworkCapabiltiesManager {
	return &NetworkCapabiltiesManager{
		Orchestrators:      make(map[string]OrchestratorInfo),
		SupportedPipelines: make(map[string]map[string]SupportedPipelineStats),
	}
}

// AddOrchestratorPipeline adds or updates a pipeline entry for an orchestrator.
func (ncm *NetworkCapabiltiesManager) AddOrchestratorPipeline(orchAddress string, pipelineType string) {
	if _, exists := ncm.Orchestrators[orchAddress]; !exists {
		ncm.Orchestrators[orchAddress] = OrchestratorInfo{
			Pipelines: make(map[string]map[string]OrchestratorPipeline),
		}
	}

	if _, exists := ncm.Orchestrators[orchAddress].Pipelines[pipelineType]; !exists {
		ncm.Orchestrators[orchAddress].Pipelines[pipelineType] = make(map[string]OrchestratorPipeline)
	}
}

// AddSupportedPipeline adds or updates a supported pipeline with cold and warm stats.
func (ncm *NetworkCapabiltiesManager) AddSupportedPipeline(pipelineType string, modelName string) {
	if _, exists := ncm.SupportedPipelines[pipelineType]; !exists {
		ncm.SupportedPipelines[pipelineType] = make(map[string]SupportedPipelineStats)
	}
	if _, exists := ncm.SupportedPipelines[pipelineType][modelName]; !exists {
		ncm.SupportedPipelines[pipelineType][modelName] = SupportedPipelineStats{
			Cold: 0,
			Warm: 0,
		}
	}
}

// IncrementSupportedPipelineCold increments the Cold value for a supported pipeline.
func (ncm *NetworkCapabiltiesManager) IncrementSupportedPipelineCold(pipelineType, modelName string) error {
	if pipelineModels, exists := ncm.SupportedPipelines[pipelineType]; exists {
		if stats, exists := pipelineModels[modelName]; exists {
			stats.Cold++
			ncm.SupportedPipelines[pipelineType][modelName] = stats
			return nil
		}
		return errors.New("model not found")
	}
	return errors.New("pipeline type not found")
}

// IncrementSupportedPipelineWarm increments the Warm value for a supported pipeline.
func (ncm *NetworkCapabiltiesManager) IncrementSupportedPipelineWarm(pipelineType, modelName string) error {
	if pipelineModels, exists := ncm.SupportedPipelines[pipelineType]; exists {
		if stats, exists := pipelineModels[modelName]; exists {
			stats.Warm++
			ncm.SupportedPipelines[pipelineType][modelName] = stats
			return nil
		}
		return errors.New("model not found")
	}
	return errors.New("pipeline type not found")
}

// AddOrchestratorPipelineModel adds or updates a model in an orchestrator's pipeline.
func (ncm *NetworkCapabiltiesManager) AddOrchestratorPipelineModel(orchAddress, pipelineType, modelName string, warm bool) error {
	if orch, exists := ncm.Orchestrators[orchAddress]; exists {
		if _, exists := orch.Pipelines[pipelineType]; !exists {
			orch.Pipelines[pipelineType] = make(map[string]OrchestratorPipeline)
		}
		orch.Pipelines[pipelineType][modelName] = OrchestratorPipeline{
			Warm: warm,
		}
		ncm.Orchestrators[orchAddress] = orch
		return nil
	}
	return errors.New("orchestrator not found")
}

// PrintJSONData is a utility function to print the current JSONData.
func (ncm *NetworkCapabiltiesManager) PrintJSONData() {
	fmt.Printf("Orchestrators: %+v\n", ncm.Orchestrators)
	fmt.Printf("Supported Pipelines: %+v\n", ncm.SupportedPipelines)
}

func buildNetworkCapabilitiesManager(livepeerNode *core.LivepeerNode) (*NetworkCapabiltiesManager, error) {
	caps := core.NewCapabilities(core.DefaultCapabilities(), nil)
	caps.SetPerCapabilityConstraints(nil)
	ods, err := livepeerNode.OrchestratorPool.GetOrchestrators(context.Background(), 100, newSuspender(), caps, common.ScoreAtLeast(0))
	caps.SetMinVersionConstraint(livepeerNode.Capabilities.MinVersionConstraint())
	if err != nil {
		return nil, err
	}

	networkCapsMgr := NewNetworkCapabilitiesManager()
	remoteInfos := ods.GetRemoteInfos()

	glog.V(common.SHORT).Infof("getting network capabilities for %d orchestrators", len(remoteInfos))

	for idx, orch_info := range remoteInfos {
		glog.V(common.DEBUG).Infof("getting capabilities for orchestrator %d %v", idx, orch_info.Transcoder)

		//ensure the orch has the proper onchain TicketParams to ensure ethAddress was configured.
		tp := orch_info.TicketParams
		var orchAddr string
		if tp != nil {
			ethAddress := tp.Recipient
			orchAddr = hexutil.Encode(ethAddress)
		} else {
			orchAddr = "0x0000000000000000000000000000000000000000"
		}

		//parse the capabilities and capacities
		if orch_info.GetCapabilities() != nil {
			for capability, constraints := range orch_info.Capabilities.Constraints.PerCapability {
				capName, err := core.CapabilityToName(core.Capability(int(capability)))
				networkCapsMgr.AddOrchestratorPipeline(orchAddr, capName)
				if err != nil {
					continue
				}

				models := constraints.GetModels()
				for model, constraint := range models {
					networkCapsMgr.AddSupportedPipeline(capName, model)
					err := networkCapsMgr.AddOrchestratorPipelineModel(orchAddr, capName, model, constraint.GetWarm())
					if err != nil {
						glog.V(common.DEBUG).Infof("  error adding model to orch %v", err)
					}
					if constraint.GetWarm() {
						err := networkCapsMgr.IncrementSupportedPipelineWarm(capName, model)
						if err != nil {
							glog.V(common.DEBUG).Infof("  error incrementing warm %v", err)
						}
					} else {
						err := networkCapsMgr.IncrementSupportedPipelineCold(capName, model)
						if err != nil {
							glog.V(common.DEBUG).Infof("  error incrementing cold %v", err)
						}
					}
					supportedCapability := networkCapsMgr.SupportedPipelines[capName]
					supportedCapabilityModel := networkCapsMgr.SupportedPipelines[capName][model]

					glog.V(common.DEBUG).Infof("  FOUND supported pipeline [%s] for model [%s]", supportedCapability, supportedCapabilityModel)
				}
			}
		}
	}
	return networkCapsMgr, nil
}
