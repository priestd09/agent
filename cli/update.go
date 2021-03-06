package cli

import (
	"os"
	"os/exec"

	"github.com/subutai-io/agent/lib/container"
	"github.com/subutai-io/agent/log"
	"strings"
	"github.com/subutai-io/agent/lib/common"
)

// Update operation can be divided into two different types: container updates and Resource Host updates.
//
// Container updates simply perform apt-get update and upgrade operations inside target containers without any extra commands.
// Since SS Management is just another container, the Subutai update command works fine with the management container too.
//
// The second type of update, a Resource Host update, checks the Ubuntu Store and compares available snap packages with those currently installed in the system and,
// if a newer version is found, installs it. Please note, system security policies requires that such commands should be performed by the superuser manually,
// otherwise an application's attempt to update itself will be blocked.
func Update(name string, check bool) {
	lock, err := common.LockFile(name, "update")
	if err != nil {
		log.Error("Another update process is already running")
	}
	defer lock.Unlock()

	if name == "rh" {
		updateRH(check)
	} else {
		updateContainer(name, check)
	}
}

func updateRH(check bool) {

	output, err := exec.Command("apt-get", "-qq", "update", "-y", "--force-yes", "-o", "Acquire::http::Timeout=5").CombinedOutput()
	log.Check(log.FatalLevel, "Updating apt index "+string(output), err)
	output, err = exec.Command("apt-get", "-qq", "dist-upgrade", "-y", "--force-yes", "-o", "Acquire::http::Timeout=5", "-s").CombinedOutput()
	log.Check(log.FatalLevel, "Checking for available update "+string(output), err)
	if len(output) == 0 {
		log.Info("No update is available")
		return
	} else if check {
		log.Info("Update is available")
		return
	}

	cmd := exec.Command("dpkg", "--configure", "-a")
	cmd.Env = []string{"DEBIAN_FRONTEND=noninteractive", "PATH=" + os.Getenv("PATH")}
	output, err = cmd.CombinedOutput()
	log.Check(log.WarnLevel, "Configuring dpkg "+string(output), err)

	cmd = exec.Command("apt-get", "-qq", "dist-upgrade", "-y", "-o", "Acquire::http::Timeout=5", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold")
	cmd.Env = []string{"DEBIAN_FRONTEND=noninteractive", "PATH=" + os.Getenv("PATH")}
	output, err = cmd.CombinedOutput()
	log.Check(log.FatalLevel, "Updating host "+string(output), err)
}

func updateContainer(name string, check bool) {
	if !container.LxcInstanceExists(name) {
		log.Error("no such instance \"" + name + "\"")
	}
	output, err := container.AttachExec(name, []string{"apt-get", "-qq", "update", "-y", "--force-yes", "-o", "Acquire::http::Timeout=5"})
	log.Check(log.FatalLevel, "Updating apt index "+strings.Join(output, "\n"), err)
	output, err = container.AttachExec(name, []string{"apt-get", "-qq", "upgrade", "-y", "--force-yes", "-o", "Acquire::http::Timeout=5", "-s"})
	log.Check(log.FatalLevel, "Checking for available update "+strings.Join(output, "\n"), err)
	if len(output) == 0 {
		log.Info("No update is available")
		return
	} else if check {
		log.Info("Update is available")
		return
	}
	output, err = container.AttachExec(name, []string{"dpkg", "--configure", "-a"}, []string{"DEBIAN_FRONTEND=noninteractive"})
	log.Check(log.FatalLevel, "Configuring dpkg "+strings.Join(output, "\n"), err)
	output, err = container.AttachExec(name, []string{"apt-get", "-qq", "upgrade", "-y", "--allow-unauthenticated", "-o", "Acquire::http::Timeout=5", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"},
		[]string{"DEBIAN_FRONTEND=noninteractive"})
	log.Check(log.FatalLevel, "Updating container "+strings.Join(output, "\n"), err)
}
