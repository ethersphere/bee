# Docker compose

The docker-compose provides an app container for Bee.
To prepare your machine to run docker compose execute
```
mkdir -p bee && cd bee
wget -q https://raw.githubusercontent.com/ethersphere/bee/master/packaging/docker/docker-compose.yml
wget -q https://raw.githubusercontent.com/ethersphere/bee/master/packaging/docker/env -O .env
```
Set all configuration variables inside `.env`

If you want to run node in full mode, set `BEE_FULL_NODE=true`

Bee requires an Ethereum endpoint to function. Obtain a free Infura account and set:
- `BEE_BLOCKCHAIN_RPC_ENDPOINT=wss://sepolia.infura.io/ws/v3/xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`

Set bee password by either setting `BEE_PASSWORD` or `BEE_PASSWORD_FILE`

If you want to use password file set it to
- `BEE_PASSWORD_FILE=/password`

Mount password file local file system by adding
```
- ./password:/password
```
to bee volumes inside `docker-compose.yml`

Start it with
```
docker-compose up -d
```

From logs find URL line with `on sepolia you can get both sepolia eth and sepolia bzz from` and prefund your node
```
docker-compose logs -f bee-1
```

Update services with
```
docker-compose pull && docker-compose up -d
```

## Running multiple Bee nodes
It is easy to run multiple bee nodes with docker compose by adding more services to `docker-compose.yaml`
To do so, open `docker-compose.yaml`, copy lines 4-54 and past this after line 54 (whole bee-1 section).
In the copied lines, replace all occurrences of `bee-1` with `bee-2` and adjust the `API_ADDR` and `P2P_ADDR` to respectively `1733`, `1734.`
Lastly, add your newly configured services under `volumes` (last lines), such that it looks like:
```yaml
volumes:
  bee-1:
  bee-2:
```

If you want to create more than two nodes, simply repeat the process above, ensuring that you keep unique name for your bee services and update the ports
