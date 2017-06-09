package cli

import (
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"fmt"

	"github.com/subutai-io/agent/config"
	"github.com/subutai-io/agent/db"
	"github.com/subutai-io/agent/lib/fs"
	"github.com/subutai-io/agent/lib/gpg"
	ovs "github.com/subutai-io/agent/lib/net"
	"github.com/subutai-io/agent/log"
)

func MapPort(protocol, internal, external, policy, domain, cert string, list, remove, sslbcknd bool) {
	if list {
		for _, v := range mapList(protocol) {
			fmt.Println(v)
		}
		return
	}

	if protocol != "tcp" && protocol != "udp" && protocol != "http" && protocol != "https" {
		log.Error("Unsupported protocol \"" + protocol + "\"")
	} else if protocol == "tcp" || protocol == "udp" {
		domain = protocol
	}

	switch {
	case (protocol == "http" || protocol == "https") && len(domain) == 0:
		log.Error("\"-d domain\" is mandatory for http protocol")
	case remove:
		mapRemove(protocol, external, domain, internal)
	case protocol == "https" && (len(cert) == 0 || !gpg.ValidatePem(cert)):
		log.Error("\"-c certificate\" is missing or invalid pem file")
	case len(internal) != 0 && !ovs.ValidSocket(internal):
		log.Error("Invalid internal socket \"" + internal + "\"")
	case (external == "8443" || external == "8444" || external == "8086") &&
		internal != "10.10.10.1:"+external:
		log.Error("Reserved system ports")
	case len(internal) != 0:
		// check external port and create nginx config
		if portIsNew(protocol, internal, domain, &external) {
			newConfig(protocol, external, domain, cert, sslbcknd)
		}

		// add containers to backend
		addLine(config.Agent.DataPrefix+"nginx-includes/"+protocol+"/"+external+"-"+domain+".conf",
			"#Add new host here", "	server "+internal+";", false)

		// save information to database
		saveMapToDB(protocol, external, domain, internal)
		containerMapToDB(protocol, external, domain, internal)
		balanceMethod(protocol, external, domain, policy)

		log.Info(ovs.GetIp() + ":" + external)
	case len(policy) != 0:
		balanceMethod(protocol, external, domain, policy)
	}
	restart()
}

func mapList(protocol string) (list []string) {
	bolt, err := db.New()
	log.Check(log.ErrorLevel, "Openning portmap database to get list", err)
	switch protocol {
	case "tcp", "udp", "http", "https":
		list = bolt.PortmapList(protocol)
	default:
		for _, v := range []string{"tcp", "udp", "http", "https"} {
			list = append(list, bolt.PortmapList(v)...)
		}
	}
	log.Check(log.WarnLevel, "Closing database", bolt.Close())
	return
}

func mapRemove(protocol, external, domain, internal string) {
	bolt, err := db.New()
	log.Check(log.ErrorLevel, "Openning portmap database to remove mapping", err)
	defer bolt.Close()
	if !bolt.PortInMap(protocol, external, domain, internal) {
		return
	}
	log.Debug("Removing mapping: " + protocol + " " + external + " " + domain + " " + internal)

	if bolt.PortMapDelete(protocol, external, domain, internal) > 0 {
		if strings.Contains(internal, ":") {
			internal = internal + ";"
		} else {
			internal = internal + ":"
		}
		addLine(config.Agent.DataPrefix+"nginx-includes/"+protocol+"/"+external+"-"+domain+".conf",
			"server "+internal, " ", true)
	} else {
		if bolt.PortMapDelete(protocol, external, domain, "") == 0 {
			bolt.PortMapDelete(protocol, external, "", "")
		}
		os.Remove(config.Agent.DataPrefix + "nginx-includes/" + protocol + "/" + external + "-" + domain + ".conf")
		if protocol == "https" {
			os.Remove(config.Agent.DataPrefix + "web/ssl/https-" + external + "-" + domain + ".key")
			os.Remove(config.Agent.DataPrefix + "web/ssl/https-" + external + "-" + domain + ".crt")
		}
	}
}

func isFree(protocol, port string) (res bool) {
	switch protocol {
	case "tcp", "http", "https":
		if ln, err := net.Listen("tcp", ovs.GetIp()+":"+port); err == nil {
			ln.Close()
			res = true
		}
	case "udp":
		if addr, err := net.ResolveUDPAddr("udp", ovs.GetIp()+":"+port); err == nil {
			if ln, err := net.ListenUDP("udp", addr); err == nil {
				ln.Close()
				res = true
			}
		}
	}
	return
}

func random(min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max-min) + min
}

func portIsNew(protocol, internal, domain string, external *string) (new bool) {
	if len(*external) != 0 {
		if port, err := strconv.Atoi(*external); err != nil || port < 1000 || port > 65536 {
			log.Error("Parameter \"external\" should be integer in range of 1000-65536")
		}
		if isFree(protocol, *external) {
			return true
		}

		bolt, err := db.New()
		log.Check(log.ErrorLevel, "Opening portmap database to read existing mappings", err)
		if !bolt.PortInMap(protocol, *external, "", "") {
			log.Error("Port is busy")
		} else if bolt.PortInMap(protocol, *external, domain, internal) {
			log.Error("Map is already exists")
		}
		new = !bolt.PortInMap(protocol, *external, domain, "")
		log.Check(log.WarnLevel, "Closing database", bolt.Close())
	} else {
		for *external = strconv.Itoa(random(1000, 65536)); !isFree(protocol, *external); *external = strconv.Itoa(random(1000, 65536)) {
			continue
		}
		new = true
	}
	return new
}

func newConfig(protocol, port, domain, cert string, sslbcknd bool) {
	log.Check(log.WarnLevel, "Creating nginx include folder",
		os.MkdirAll(config.Agent.DataPrefix+"nginx-includes/"+protocol, 0755))
	conf := config.Agent.DataPrefix + "nginx-includes/" + protocol + "/" + port + "-" + domain + ".conf"

	switch protocol {
	case "https":
		log.Check(log.ErrorLevel, "Creating certificate dirs", os.MkdirAll(config.Agent.DataPrefix+"/web/ssl/", 0755))
		fs.Copy(config.Agent.AppPrefix+"etc/nginx/tmpl/vhost-ssl.example", conf)
		addLine(conf, "return 301 https://$host$request_uri;  # enforce https", "	    return 301 https://$host:"+port+"$request_uri;  # enforce https", true)
		addLine(conf, "listen	443;", "	listen "+port+";", true)
		addLine(conf, "server_name DOMAIN;", "server_name "+domain+";", true)
		if sslbcknd {
			addLine(conf, "proxy_pass http://DOMAIN-upstream/;", "	proxy_pass https://https-"+port+"-"+domain+";", true)
		} else {
			addLine(conf, "proxy_pass http://DOMAIN-upstream/;", "	proxy_pass http://https-"+port+"-"+domain+";", true)
		}
		addLine(conf, "upstream DOMAIN-upstream {", "upstream https-"+port+"-"+domain+" {", true)

		crt, key := gpg.ParsePem(cert)
		log.Check(log.WarnLevel, "Writing certificate body", ioutil.WriteFile(config.Agent.DataPrefix+"web/ssl/https-"+port+"-"+domain+".crt", crt, 0644))
		log.Check(log.WarnLevel, "Writing key body", ioutil.WriteFile(config.Agent.DataPrefix+"web/ssl/https-"+port+"-"+domain+".key", key, 0644))

		addLine(conf, "ssl_certificate /var/snap/subutai/current/web/ssl/UNIXDATE.crt;",
			"ssl_certificate "+config.Agent.DataPrefix+"web/ssl/https-"+port+"-"+domain+".crt;", true)
		addLine(conf, "ssl_certificate_key /var/snap/subutai/current/web/ssl/UNIXDATE.key;",
			"ssl_certificate_key "+config.Agent.DataPrefix+"web/ssl/https-"+port+"-"+domain+".key;", true)
	case "http":
		fs.Copy(config.Agent.AppPrefix+"etc/nginx/tmpl/vhost.example", conf)
		addLine(conf, "listen 	80;", "	listen "+port+";", true)
		addLine(conf, "return 301 http://$host$request_uri;", "	    return 301 http://$host:"+port+"$request_uri;", true)
		addLine(conf, "server_name DOMAIN;", "server_name "+domain+";", true)
		addLine(conf, "proxy_pass http://DOMAIN-upstream/;", "	proxy_pass http://http-"+port+"-"+domain+";", true)
		addLine(conf, "upstream DOMAIN-upstream {", "upstream http-"+port+"-"+domain+" {", true)
	case "tcp":
		fs.Copy(config.Agent.AppPrefix+"etc/nginx/tmpl/stream.example", conf)
		addLine(conf, "listen PORT;", "	listen "+port+";", true)
	case "udp":
		fs.Copy(config.Agent.AppPrefix+"etc/nginx/tmpl/stream.example", conf)
		addLine(conf, "listen PORT;", "	listen "+port+" udp;", true)
	}
	addLine(conf, "server localhost:81;", " ", true)
	addLine(conf, "upstream PROTO-PORT {", "upstream "+protocol+"-"+port+"-"+domain+" {", true)
	addLine(conf, "proxy_pass PROTO-PORT;", "	proxy_pass "+protocol+"-"+port+"-"+domain+";", true)
}

func balanceMethod(protocol, port, domain, policy string) {
	replaceString := "upstream " + protocol + "-" + port + "-" + domain + " {"
	replace := false
	bolt, err := db.New()
	log.Check(log.ErrorLevel, "Openning portmap database to check if port is mapped", err)
	if !bolt.PortInMap(protocol, port, domain, "") {
		log.Error("Port is not mapped")
	}
	switch policy {
	case "round-robin", "round_robin":
		policy = "#round-robin"
	//  "least_conn":
	case "least_time":
		if protocol == "tcp" {
			policy = policy + " connect"
		} else {
			policy = policy + " header"
			log.Warn("This policy is not supported in http upstream")
			return
		}
	case "hash":
		policy = policy + " $remote_addr"
	case "ip_hash":
		if protocol != "http" {
			log.Warn("ip_hash policy allowed only for http protocol")
			return
		}
	default:
		log.Debug("Unsupported balancing method \"" + policy + "\", ignoring")
		return
	}

	if p := bolt.GetMapMethod(protocol, port, domain); len(p) != 0 && p != policy {
		replaceString = "; #policy"
		replace = true
	} else if p == policy {
		return
	}
	log.Check(log.WarnLevel, "Saving map method", bolt.SetMapMethod(protocol, port, domain, policy))
	log.Check(log.WarnLevel, "Closing database", bolt.Close())

	addLine(config.Agent.DataPrefix+"nginx-includes/"+protocol+"/"+port+"-"+domain+".conf",
		replaceString, "	"+policy+"; #policy", replace)
}

func saveMapToDB(protocol, external, domain, internal string) {
	bolt, err := db.New()
	log.Check(log.ErrorLevel, "Openning database to save portmap", err)
	if !bolt.PortInMap(protocol, external, domain, internal) {
		log.Check(log.WarnLevel, "Saving port map to database", bolt.PortMapSet(protocol, external, domain, internal))
	}
	log.Check(log.WarnLevel, "Closing database", bolt.Close())
}

func containerMapToDB(protocol, external, domain, internal string) {
	bolt, err := db.New()
	log.Check(log.ErrorLevel, "Openning database to add portmap to container", err)
	for _, name := range bolt.ContainerByKey("ip", strings.Split(internal, ":")[0]) {
		bolt.ContainerMapping(name, protocol, external, domain, internal)
	}
	log.Check(log.WarnLevel, "Closing database", bolt.Close())
}