docker-compose  up vault  -d  && \
sleep 3 && \
vault-addr  redis_user="default" redis_password="" kafka_user="" kafka_password="" redis_db=0 && \\
docker-compose up zookeeper -d && \\
docker-compose up kafka-1 -d  && \\
sleep 5 && \
/Users/jc/Downloads/kafka_2.13-3.7.0/bin/kafka-topics.sh --bootstrap-server localhost:9092 --topi
c transformed-weather-data --create --partitions 1 --replication-factor 1 && \\
sleep 2 && \
/Users/jc/Downloads/kafka_2.13-3.7.0/bin/kafka-topics.sh --bootstrap-server localhost:9092 --topic raw-weather-report --create --partitions 1 --replication-factor 1 && \\
docker-compose up -d && \\
docker-compose logs -f

