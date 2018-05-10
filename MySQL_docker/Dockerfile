# Dockerfile for Python, Go and Ansible with the deployment engine, MySQL in a parallel container.
# WIP = work in progress
FROM mysql:5.7.22

# Environment variables
ENV MYSQL_ROOT_PASSWORD mypassword
COPY ./k8sql.sql /docker-entrypoint-initdb.d/