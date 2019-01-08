package helpers

import (
	"fmt"
	"strings"

	"github.com/hashicorp/nomad/api"
	"github.com/schollz/progressbar"
	log "github.com/sirupsen/logrus"
	"gopkg.in/urfave/cli.v1"
)

var (
	nodeCache = make(map[string]*api.Node)
)

func FilteredClientList(client *api.Client, c *cli.Context) ([]*api.NodeListStub, error) {
	log.Info("Finding legible nodes")
	nodes, _, err := client.Nodes().List(&api.QueryOptions{Prefix: c.String("filter-prefix")})
	if err != nil {
		return nil, err
	}

	bar := progressbar.New(len(nodes))
	if !c.Bool("no-progress") {
		bar.RenderBlank()
		defer func() {
			bar.Finish()
		}()
	}

	matches := make([]*api.NodeListStub, 0)
	for _, node := range nodes {
		// only consider nodes that is ready
		if node.Status != "ready" {
			log.Debugf("Node %s is not in status=ready (%s)", node.Name, node.Status)
			goto NEXT_NODE
		}

		// only consider nodes with the right node class
		if class := c.String("filter-class"); class != "" && node.NodeClass != class {
			log.Debugf("Node %s class '%s' do not match expected value '%s'", node.Name, node.NodeClass, class)
			goto NEXT_NODE
		}

		// only consider nodes with the right nomad version
		if version := c.String("filter-version"); version != "" && node.Version != version {
			log.Debugf("Node %s version '%s' do not match expected node version '%s'", node.Name, node.Version, version)
			goto NEXT_NODE
		}

		// only consider nodes with the right eligibility
		if eligibility := c.String("filter-eligibility"); eligibility != "" && node.SchedulingEligibility != eligibility {
			log.Debugf("Node %s eligibility '%s' do not match expected node eligibility '%s'", node.Name, node.SchedulingEligibility, eligibility)
			goto NEXT_NODE
		}

		// filter by client meta keys
		if meta := c.StringSlice("filter-meta"); len(meta) > 0 {
			for _, chunk := range meta {
				split := strings.Split(chunk, "=")
				if len(split) != 2 {
					return nil, fmt.Errorf("Could not marge filter-meta '%s' as 'key=value' pair", chunk)
				}
				key := split[0]
				value := split[1]

				if nodeValue := getNodeMetaProperty(node.ID, key, client); nodeValue != value {
					log.Debugf("Node %s Meta key '%s' value '%s' do not match expected '%s'", node.Name, key, nodeValue, value)
					goto NEXT_NODE
				}
			}
		}

		// filter by client attribute keys
		if meta := c.StringSlice("filter-attribute"); len(meta) > 0 {
			for _, chunk := range meta {
				split := strings.Split(chunk, "=")
				if len(split) != 2 {
					return nil, fmt.Errorf("Could not marge filter-meta '%s' as 'key=value' pair", chunk)
				}
				key := split[0]
				value := split[1]

				if nodeValue := getNodeAttributesProperty(node.ID, key, client); nodeValue != value {
					log.Debugf("Node %s Attribute key '%s' value '%s' do not match expected '%s'", node.Name, key, nodeValue, value)
					goto NEXT_NODE
				}
			}
		}

		// continue to furhter processing
		log.Debugf("Node %s passed all all filters", node.Name)
		matches = append(matches, node)
		goto NEXT_NODE

	NEXT_NODE:
		if !c.Bool("no-progress") {
			bar.Add(1)
		}
		continue
	}

	if !c.Bool("no-progress") {
		bar.Finish()
		fmt.Println()
	}

	log.Infof("Found %d matched nodes", len(matches))

	// only work on specific percent of nodes
	if percent := c.Int("percent"); percent < 100 {
		log.Infof("Only %d percent of nodes should be used", percent)
		matches = matches[0:len(matches) * percent / 100]
	}

	// noop mode will fail the matching to prevent any further processing
	if c.BoolT("noop") {
		for _, node := range matches {
			log.Infof("Node %s matched!", node.Name)
		}
		return nil, fmt.Errorf("noop mode, aborting")
	}

	return matches, nil
}

func hasFilter(c *cli.Context, field string) bool {
	return c.String(field) != ""
}

func getNodeMetaProperty(nodeID string, key string, client *api.Client) string {
	node, err := lookupNode(nodeID, client)
	if err != nil {
		log.Errorf("Could not lookup the node in Nomad API: %s", err)
		return ""
	}

	// spew.Dump(node)
	d, ok := node.Meta[key]
	if !ok {
		return "__not_found__"
	}
	return d
}

func getNodeAttributesProperty(nodeID string, key string, client *api.Client) string {
	node, err := lookupNode(nodeID, client)
	if err != nil {
		log.Errorf("Could not lookup the node in Nomad API: %s", err)
		return ""
	}

	// spew.Dump(node)
	d, ok := node.Attributes[key]
	if !ok {
		return "__not_found__"
	}
	return d
}

func lookupNode(nodeID string, client *api.Client) (*api.Node, error) {
	data, ok := nodeCache[nodeID]
	if !ok {
		node, _, err := client.Nodes().Info(nodeID, nil)
		if err != nil {
			return nil, err
		}

		nodeCache[nodeID] = node
		return node, nil
	}

	return data, nil

}
