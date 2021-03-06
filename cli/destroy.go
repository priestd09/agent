package cli

import (
	"strings"

	"github.com/subutai-io/agent/db"
	"github.com/subutai-io/agent/lib/container"
	"github.com/subutai-io/agent/lib/gpg"
	"github.com/subutai-io/agent/lib/net"
	"github.com/subutai-io/agent/lib/net/p2p"
	"github.com/subutai-io/agent/lib/template"
	"github.com/subutai-io/agent/log"
	"github.com/subutai-io/agent/agent/utils"
)

// LxcDestroy simply removes every resource associated with a Subutai container or template:
// data, network, configs, etc.
//
// The destroy command always runs each step in "force" mode to provide reliable deletion results;
// even if some instance components were already removed, the destroy command will continue to perform all operations
// once again while ignoring possible underlying errors: i.e. missing configuration files.
func LxcDestroy(id string, vlan bool, ignoreMissing bool) {
	var msg string
	if len(id) == 0 {
		log.Error("Please specify container/template name or vlan id")
	}

	if strings.HasPrefix(id, "id:") {
		contId := strings.ToUpper(strings.TrimPrefix(id, "id:"))
		for _, c := range container.Containers() {
			if contId == gpg.GetFingerprint(c) {
				LxcDestroy(c, false, false)
				return
			}
		}
		log.Error("Container " + contId + " not found")
	} else if vlan {
		list, err := db.INSTANCE.ContainerByKey("vlan", id)
		if !log.Check(log.WarnLevel, "Reading container metadata from db", err) {
			for _, c := range list {
				LxcDestroy(c, false, true)
			}
			msg = "Vlan " + id + " is destroyed"
		}
		cleanupNet(id)
	} else if id != "everything" {
		if container.IsTemplate(id) {
			LxcDestroyTemplate(id)
			return
		}

		c, err := db.INSTANCE.ContainerByName(id)
		log.Check(log.WarnLevel, "Reading container metadata from db", err)

		msg = "Container " + id + " is destroyed"

		if len(c) != 0 {

			if ip, ok := c["ip"]; ok {
				if vlan, ok := c["vlan"]; ok {
					ProxyDel(vlan, ip, false)
				}
			}

			removePortMap(id)

			net.DelIface(c["interface"])

			log.Check(log.ErrorLevel, "Destroying container", container.DestroyContainer(id))

		} else if container.IsContainer(id) {

			err = container.DestroyContainer(id)

			log.Check(log.ErrorLevel, "Destroying container", err)
		} else if !ignoreMissing {
			log.Error(id + " not found")
		}

	}

	if id == "everything" {
		list, err := db.INSTANCE.ContainerList()
		if !log.Check(log.WarnLevel, "Reading container metadata from db", err) {
			for _, name := range list {
				LxcDestroy(name, false, true)
				c, err := db.INSTANCE.ContainerByName(name)
				if !log.Check(log.WarnLevel, "Reading container metadata from db", err) {
					if v, ok := c["vlan"]; ok {
						cleanupNet(v)
					}
				}
			}
		}
		msg = id + " is destroyed"
	}

	if id == "management" {
		template.MngDel()
	}

	if len(msg) == 0 {
		msg = id + " not found. Please check if the name is correct"
	}

	log.Info(msg)
}

func LxcDestroyTemplate(name string) {
	container.DestroyTemplate(name)
}

func cleanupNet(id string) {
	net.DelIface("gw-" + id)
	p2p.RemoveByIface("p2p" + id)
	cleanupNetStat(id)
	ProxyDel(id, "", true)
}

// cleanupNetStat drops data from database about network trafic for specified VLAN
func cleanupNetStat(vlan string) {
	c, err := utils.InfluxDbClient()
	if err == nil {
		defer c.Close()
	}
	queryInfluxDB(c, `drop series from host_net where iface = 'p2p`+vlan+`'`)
	queryInfluxDB(c, `drop series from host_net where iface = 'gw-`+vlan+`'`)
}

func removePortMap(name string) {
	if portMap, err := db.INSTANCE.GetContainerMapping(name);
		!log.Check(log.WarnLevel, "Reading container metadata from db", err) {
		for _, v := range portMap {
			MapPort(v["protocol"], v["internal"], v["external"], "", v["domain"], "", false, true, false)
		}
	}
}

func Prune() {
	var templatesInUse []string

	//filter out all templates that have child containers
	for _, c := range container.Containers() {
		cont := c

		self := strings.TrimSpace(container.GetProperty(cont, "subutai.template")) + ":" +
			strings.TrimSpace(container.GetProperty(cont, "subutai.template.owner")) + ":" +
			strings.TrimSpace(container.GetProperty(cont, "subutai.template.version"))

		parent := strings.TrimSpace(container.GetProperty(cont, "subutai.parent")) + ":" +
			strings.TrimSpace(container.GetProperty(cont, "subutai.parent.owner")) + ":" +
			strings.TrimSpace(container.GetProperty(cont, "subutai.parent.version"))

		for self != parent || container.IsContainer(cont) {
			templatesInUse = append(templatesInUse, parent)

			cont = parent

			self = strings.TrimSpace(container.GetProperty(cont, "subutai.template")) + ":" +
				strings.TrimSpace(container.GetProperty(cont, "subutai.template.owner")) + ":" +
				strings.TrimSpace(container.GetProperty(cont, "subutai.template.version"))

			parent = strings.TrimSpace(container.GetProperty(cont, "subutai.parent")) + ":" +
				strings.TrimSpace(container.GetProperty(cont, "subutai.parent.owner")) + ":" +
				strings.TrimSpace(container.GetProperty(cont, "subutai.parent.version"))
		}

	}

	//figure out unused templates
	unusedTemplates := difference(container.Templates(), templatesInUse)

	//remove unused templates
	for _, t := range unusedTemplates {
		log.Info("Destroying " + t)
		container.DestroyTemplate(t)
	}

}

func difference(a, b []string) []string {
	mb := map[string]bool{}
	for _, x := range b {
		mb[x] = true
	}
	var ab []string
	for _, x := range a {
		if _, ok := mb[x]; !ok {
			ab = append(ab, x)
		}
	}
	return ab
}
