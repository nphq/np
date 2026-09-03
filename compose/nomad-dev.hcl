# Nomad dev-agent client plugin config (used by compose.yml).
# Points the docker driver at the host Docker socket so container jobs can run
# from inside the nomad container (Docker-in-Docker style).
plugin "docker" {
  config {
    endpoint = "unix:///var/run/docker.sock"
  }
}
