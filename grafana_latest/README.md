# Grafana v11 migration

To develop try and test grafana v11 

1. uncomment the grafana latest part on docker-compose
```
 grafana_latest:
    image: grafana/grafana:latest
    user: "0:0"
    environment:
      GF_DATABASE_TYPE: postgres
      GF_DATABASE_HOST: postgres:5432
      GF_DATABASE_NAME: pgwatch3_grafana
      GF_DATABASE_USER: pgwatch3
      GF_DATABASE_PASSWORD: pgwatch3admin
      GF_DATABASE_SSL_MODE: disable
      GF_AUTH_ANONYMOUS_ENABLED: true
      GF_DASHBOARDS_DEFAULT_HOME_DASHBOARD_PATH: /var/lib/grafana/dashboards/1-global-db-overview.json
      GF_INSTALL_PLUGINS: marcusolsson-treemap-panel
    ports:
      - "3001:3000"
    restart: unless-stopped
    volumes:
      - "./grafana_latest/postgres_datasource.yml:/etc/grafana/provisioning/datasources/pg_ds.yml"
      - "./grafana_latest/postgres_dashboard.yml:/etc/grafana/provisioning/dashboards/pg_db.yml"
      - "./grafana_latest/postgres/v11:/var/lib/grafana/dashboards"
    depends_on:
      postgres:
        condition: service_healthy
```

2. comment grafana v10 
````
  # grafana:
  #   image: grafana/grafana:latest
  #   user: "0:0"
  #   environment:
  #     GF_DATABASE_TYPE: postgres
  #     GF_DATABASE_HOST: postgres:5432
  #     GF_DATABASE_NAME: pgwatch3_grafana
  #     GF_DATABASE_USER: pgwatch3
  #     GF_DATABASE_PASSWORD: pgwatch3admin
  #     GF_DATABASE_SSL_MODE: disable
  #     GF_AUTH_ANONYMOUS_ENABLED: true
  #     GF_DASHBOARDS_DEFAULT_HOME_DASHBOARD_PATH: /var/lib/grafana/dashboards/1-global-db-overview.json
  #     GF_INSTALL_PLUGINS: marcusolsson-treemap-panel
  #   ports:
  #     - "3000:3000"
  #   restart: unless-stopped
  #   volumes:
  #     - "./grafana/postgres_datasource.yml:/etc/grafana/provisioning/datasources/pg_ds.yml"
  #     - "./grafana/postgres_dashboard.yml:/etc/grafana/provisioning/dashboards/pg_db.yml"
  #     - "./grafana/postgres/v10:/var/lib/grafana/dashboards"
  #   depends_on:
  #     postgres:
  #       condition: service_healt
````

3. compose

`````
docker compose up -d
`````


4. you can see the v11 dashboards in localhost 3001 (this is going to be probably)