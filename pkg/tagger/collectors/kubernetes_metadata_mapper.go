// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver,kubelet

package collectors

import (
	"strings"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (c *KubeMetadataCollector) getTagInfos(pods []*kubelet.Pod) []*TagInfo {
	var err error
	metadataByNsPods := apiv1.NewNamespacesPodsStringsSet()
	if !c.clusterAgentEnabled {
		var nodeName string
		nodeName, err = c.kubeUtil.GetNodename()
		if err != nil {
			log.Errorf("Could not retrieve the Nodename, err: %v", err)
			return nil
		}
		metadataByNsPods, err = c.dcaClient.GetPodsMetadataForNode(nodeName)
		if err != nil {
			log.Errorf("Could not pull the metadata map of pods on node %s from the Datadog Cluster Agent: %s", nodeName, err.Error())
			return nil
		}
	}
	var tagInfo []*TagInfo
	var metadataNames []string
	var tag []string
	for _, po := range pods {
		if kubelet.IsPodReady(po) == false {
			log.Debugf("pod %q is not ready, skipping", po.Metadata.Name)
			continue
		}

		// We cannot define if a hostNetwork Pod is a member of a service
		if po.Spec.HostNetwork == true {
			for _, container := range po.Status.Containers {
				info := &TagInfo{
					Source:               kubeMetadataCollectorName,
					Entity:               container.ID,
					HighCardTags:         []string{},
					OrchestratorCardTags: []string{},
					LowCardTags:          []string{},
				}
				tagInfo = append(tagInfo, info)
			}
			continue
		}

		tagList := utils.NewTagList()
		if !c.clusterAgentEnabled {
			metadataNames, err = apiserver.GetPodMetadataNames(po.Spec.NodeName, po.Metadata.Namespace, po.Metadata.Name)
			if err != nil {
				log.Errorf("Could not fetch cluster level tags for the pod %s: %s", po.Metadata.Name, err.Error())
				continue
			}
		} else {
			metadataNames = metadataByNsPods[po.Metadata.Namespace][po.Metadata.Name].List()
		}
		for _, tagDCA := range metadataNames {
			log.Tracef("Tagging %s with %s", po.Metadata.Name, tagDCA)
			tag = strings.Split(tagDCA, ":")
			if len(tag) != 2 {
				continue
			}
			tagList.AddLow(tag[0], tag[1])
		}

		low, orchestrator, high := tagList.Compute()
		// Register the tags for the pod itself
		if po.Metadata.UID != "" {
			podInfo := &TagInfo{
				Source:               kubeMetadataCollectorName,
				Entity:               kubelet.PodUIDToEntityName(po.Metadata.UID),
				HighCardTags:         high,
				OrchestratorCardTags: orchestrator,
				LowCardTags:          low,
			}
			tagInfo = append(tagInfo, podInfo)
		}
		// Register the tags for all its containers
		for _, container := range po.Status.Containers {
			info := &TagInfo{
				Source:               kubeMetadataCollectorName,
				Entity:               container.ID,
				HighCardTags:         high,
				OrchestratorCardTags: orchestrator,
				LowCardTags:          low,
			}
			tagInfo = append(tagInfo, info)
		}
	}
	return tagInfo
}

// addToCacheMetadataMapping is acting like the DCA at the node level.
func (c *KubeMetadataCollector) addToCacheMetadataMapping(kubeletPodList []*kubelet.Pod) error {
	if len(kubeletPodList) == 0 {
		log.Debugf("Empty kubelet pod list")
		return nil
	}

	reachablePods := make([]*kubelet.Pod, 0)
	nodeName := ""
	for _, p := range kubeletPodList {
		if p.Status.PodIP == "" {
			continue
		}
		if nodeName == "" && p.Spec.NodeName != "" {
			nodeName = p.Spec.NodeName
		}
		reachablePods = append(reachablePods, p)
	}
	return c.apiClient.NodeMetadataMapping(nodeName, reachablePods)
}
