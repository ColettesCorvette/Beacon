package gather

import (
	"context"
	"sync"
	"time"

	"github.com/moby/moby/client"
	"github.com/pkg/errors"
	central "orbinea.example/fluxia/nexus"
)

var (
	dockerOnce sync.Once
	dockerCli  *client.Client
	dockerErr  error
)

// dockerClient retourne un client Docker partagé, créé une seule fois, au lieu
// d'en ouvrir (et fermer) un à chaque appel/tick. client.FromEnv honore
// DOCKER_HOST/DOCKER_API_VERSION comme le faisait l'ancien NewEnvClient. Le
// client vit pour toute la durée du process : ne pas le Close.
func dockerClient() (*client.Client, error) {
	dockerOnce.Do(func() {
		dockerCli, dockerErr = client.New(client.FromEnv)
	})
	return dockerCli, dockerErr
}

func ContainerStatus(name string) (central.LHServerDockerStatus, error) {
	resp := central.LHServerDockerStatus{}
	ctx := context.Background()

	cli, err := dockerClient()
	if err != nil {
		return resp, errors.Wrap(err, "failed to get docker client")
	}

	containerInspection, err := cli.ContainerInspect(ctx, name, client.ContainerInspectOptions{})
	if err != nil {
		return resp, errors.Wrap(err, "failed to inspect container")
	}

	resp.Name = name
	resp.StartDate = containerInspection.Container.Created
	resp.Image = containerInspection.Container.Image
	resp.Running = containerInspection.Container.State.Running

	return resp, nil
}

func ExecInContainer(containerName string, command []string) error {
	// Timeout global : sans lui, un dockerd qui ne répond pas bloque
	// indéfiniment ExecCreate/ExecAttach/ExecInspect (et le handler HTTP
	// appelant) — la boucle d'inspect ne borne que les itérations
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cli, err := dockerClient()
	if err != nil {
		return errors.Wrap(err, "failed to get docker client")
	}

	exec, err := cli.ExecCreate(ctx, containerName, client.ExecCreateOptions{
		User: "root",
		Cmd:  command,
	})
	if err != nil {
		return errors.Wrap(err, "failed to create exec")
	}

	attach, err := cli.ExecAttach(ctx, exec.ID, client.ExecAttachOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to attach exec")
	}
	// La connexion hijackée doit être fermée par l'appelant, sinon chaque
	// appel fuit un descripteur vers dockerd (cli.Close ne la ferme pas)
	defer attach.Close()

	// Attendre la fin réelle de l'exec et vérifier son code retour :
	// l'ancien sleep(300ms) répondait "OK" sans savoir si la commande
	// avait tourné
	for i := 0; i < 50; i++ {
		inspect, err := cli.ExecInspect(ctx, exec.ID, client.ExecInspectOptions{})
		if err != nil {
			return errors.Wrap(err, "failed to inspect exec")
		}
		if !inspect.Running {
			if inspect.ExitCode != 0 {
				return errors.Errorf("exec %v in %s exited with code %d", command, containerName, inspect.ExitCode)
			}
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.Errorf("exec %v in %s: timeout après 5s", command, containerName)
}
