# Tic-Tac-Toe Application Deployment on AWS Fargate

This README details the process of deploying the Tic-Tac-Toe Go web application (with Websockets) to AWS Fargate, including Dockerization, ECS infrastructure setup, and custom domain configuration using Route 53.

## Goal

The primary goal was to deploy the Go web application, which serves a Tic-Tac-Toe game, onto AWS Fargate. AWS Fargate is a serverless compute engine for Amazon Elastic Container Service (ECS) that allows you to run containers without having to provision, configure, or scale clusters of virtual machines. This simplifies deployment and management.

## Prerequisites

Before starting the deployment, ensure you have the following:

*   **AWS CLI configured:** The AWS Command Line Interface (CLI) must be installed and configured with appropriate credentials to interact with your AWS account.
*   **Docker installed:** Docker Desktop or Docker Engine must be installed on your local machine to build and push Docker images.

---

## Step-by-Step Deployment Process

### Step 1: Containerization of the Application

The first crucial step for deploying any application to Fargate is to containerize it. This means packaging your application and all its dependencies into a Docker image.

1.  **Reviewing the `dockerfile`:**
    *   The existing `dockerfile` was examined to understand its structure and build process.
    *   It uses a multi-stage build:
        *   **Builder Stage:** `golang:alpine` is used to compile the Go application.
        *   **Runtime Stage:** A smaller `alpine:latest` image is used, copying only the compiled binary and necessary static assets (`src`, `assets`) from the builder stage. This results in a lean and secure final image.
    *   The `EXPOSE 3000` instruction indicates that the application listens on port 3000 inside the container.

    ```dockerfile
    # Builder Stage
    FROM golang:alpine AS builder

    WORKDIR /app

    # Cache dependencies
    COPY go.mod go.sum ./
    RUN go mod download

    # Copy source code and build the application
    COPY . .
    RUN go build -o /app/bin/tictac .

    # Runtime Stage
    FROM alpine:latest

    # Add non-root user and set up the application directory
    RUN addgroup -S appgroup && \
        adduser -S appuser -G appgroup && \
        mkdir -p /app/src

    WORKDIR /app

    # Copy the binary and HTML files from the builder
    COPY --from=builder /app/bin/tictac /app/bin/tictac
    COPY --from=builder /app/src /app/src
    COPY --from=builder /app/assets /app/assets

    # Adjust permissions
    RUN chown -R appuser:appgroup /app
    USER appuser

    # Set entrypoint and expose the port
    ENTRYPOINT ["/app/bin/tictac"]
    EXPOSE 3000
    ```

2.  **Verifying Application Port:**
    *   The `main.go` file was checked to confirm that the Go application listens on port `3000` (`http.ListenAndServe(":3000", r)`). This consistency is vital for the load balancer and service to correctly route traffic.

### Step 2: Docker Image Build and Push to Amazon ECR

Once the `dockerfile` was confirmed, the next step was to build the Docker image and store it in a container registry that Fargate could access. Amazon Elastic Container Registry (ECR) is AWS's managed Docker container registry.

1.  **ECR Login:**
    *   Log in to your ECR registry using the AWS CLI and Docker. Replace `207888179522.dkr.ecr.us-east-1.amazonaws.com` with your actual ECR repository URI.
    ```bash
    aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin 207888179522.dkr.ecr.us-east-1.amazonaws.com
    ```

2.  **Docker Image Build:**
    *   Build the Docker image from your `dockerfile` and tag it with the full URI of your ECR repository. The `:latest` tag indicates it's the most recent version.
    ```bash
    docker build -t 207888179522.dkr.ecr.us-east-1.amazonaws.com/tic-tac-toe:latest .
    ```

3.  **Docker Image Push:**
    *   Push the built image to your ECR repository. This makes the image available for your Fargate tasks to pull and run.
    ```bash
    docker push 207888179522.dkr.ecr.us-east-1.amazonaws.com/tic-tac-toe:latest
    ```

### Step 3: AWS ECS Infrastructure Setup

With the Docker image ready in ECR, the necessary AWS Elastic Container Service (ECS) infrastructure was set up to run the application on Fargate.

1.  **ECS Cluster Creation:**
    *   An ECS cluster named `tic-tac-toe` was created to logically group the tasks.
    ```bash
    aws ecs create-cluster --cluster-name tic-tac-toe --region us-east-1
    ```

2.  **Task Definition Creation and Registration:**
    *   A task definition acts as a blueprint for your application, specifying the Docker image, CPU/memory, port mappings, etc.
    *   **IAM Role for Execution (`ecsTaskExecutionRole`):** This role grants ECS permissions to pull images from ECR and send logs to CloudWatch.
        ```bash
        aws iam create-role --role-name ecsTaskExecutionRole --assume-role-policy-document '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}' --region us-east-1
        aws iam attach-role-policy --role-name ecsTaskExecutionRole --policy-arn arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy --region us-east-1
        ```
    *   **IAM Role for Task (`ecsTaskRole`):** This role is required for features like `execute-command` and grants permissions for actions performed by the container itself.
        ```bash
        aws iam create-role --role-name ecsTaskRole --assume-role-policy-document '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}' --region us-east-1
        ```
    *   **`task-definition.json` content:** This file was created with the necessary configurations, including the `executionRoleArn` and `taskRoleArn`.
        ```json
        {
          "family": "tic-tac-toe",
          "taskRoleArn": "arn:aws:iam::207888179522:role/ecsTaskRole",
          "executionRoleArn": "arn:aws:iam::207888179522:role/ecsTaskExecutionRole",
          "networkMode": "awsvpc",
          "containerDefinitions": [
            {
              "name": "tic-tac-toe",
              "image": "207888179522.dkr.ecr.us-east-1.amazonaws.com/tic-tac-toe:latest",
              "portMappings": [
                {
                  "containerPort": 3000,
                  "hostPort": 3000,
                  "protocol": "tcp"
                }
              ],
              "essential": true
            }
          ],
          "requiresCompatibilities": [
            "FARGATE"
          ],
          "cpu": "256",
          "memory": "512"
        }
        ```
    *   **Register Task Definition:** The task definition was registered with ECS.
        ```bash
        aws ecs register-task-definition --cli-input-json file://task-definition.json --region us-east-1
        ```

3.  **Application Load Balancer (ALB) Setup:**
    *   **VPC and Subnet Discovery:** The default VPC ID and its subnet IDs were retrieved.
        ```bash
        aws ec2 describe-vpcs --query 'Vpcs[?IsDefault].VpcId' --output text --region us-east-1
        aws ec2 describe-subnets --filters "Name=vpc-id,Values=<your-vpc-id>" --query 'Subnets[*].SubnetId' --output json --region us-east-1
        ```
    *   **Security Group for ALB (`tic-tac-toe-lb-sg`):** A security group was created for the ALB, allowing inbound HTTP traffic on port 80 from anywhere.
        ```bash
        aws ec2 create-security-group --group-name tic-tac-toe-lb-sg --description "Tic Tac Toe LB Security Group" --vpc-id <your-vpc-id> --region us-east-1
        aws ec2 authorize-security-group-ingress --group-id <lb-sg-id> --protocol tcp --port 80 --cidr 0.0.0.0/0 --region us-east-1
        ```
    *   **ALB Creation:** The Application Load Balancer was created using the identified subnets and security group.
        ```bash
        aws elbv2 create-load-balancer --name tic-tac-toe-lb --type application --subnets <subnet-ids> --security-groups <lb-sg-id> --region us-east-1
        ```
    *   **Target Group Creation (`tic-tac-toe-tg`):** A target group was created to route traffic to the ECS tasks, with health checks configured for port 3000 and path `/`.
        ```bash
        aws elbv2 create-target-group --name tic-tac-toe-tg --protocol HTTP --port 3000 --vpc-id <your-vpc-id> --health-check-path / --target-type ip --region us-east-1
        ```
    *   **Listener Creation:** An HTTP listener on port 80 was created for the ALB, forwarding traffic to the `tic-tac-toe-tg` target group.
        ```bash
        aws elbv2 create-listener --load-balancer-arn <lb-arn> --protocol HTTP --port 80 --default-actions Type=forward,TargetGroupArn=<tg-arn> --region us-east-1
        ```

4.  **ECS Service Creation:**
    *   **Security Group for Service (`tic-tac-toe-service-sg`):** A security group was created for the ECS service, allowing inbound traffic on port 3000 only from the ALB's security group.
        ```bash
        aws ec2 create-security-group --group-name tic-tac-toe-service-sg --description "Tic Tac Toe Service Security Group" --vpc-id <your-vpc-id> --region us-east-1
        aws ec2 authorize-security-group-ingress --group-id <service-sg-id> --protocol tcp --port 3000 --source-group <lb-sg-id> --region us-east-1
        ```
    *   **Service Creation:** The ECS service was created, linking it to the cluster, task definition, desired count (1 task), Fargate launch type, network configuration, and load balancer.
        ```bash
        aws ecs create-service --cluster tic-tac-toe --service-name tic-tac-toe-service --task-definition tic-tac-toe:2 --desired-count 1 --launch-type FARGATE --network-configuration "awsvpcConfiguration={subnets=[<subnet-ids>],securityGroups=[<service-sg-id>],assignPublicIp=ENABLED}" --load-balancers "targetGroupArn=<tg-arn>,containerName=tic-tac-toe,containerPort=3000" --region us-east-1
        ```

### Step 4: Verification and Troubleshooting

After setting up the infrastructure, the deployment was verified and initial access issues were troubleshooted.

1.  **Initial Access Attempts (curl):** Attempts to access the application using the ALB's DNS name (`http://tic-tac-toe-lb-813944911.us-east-1.elb.amazonaws.com`) initially failed due to DNS propagation delays.
2.  **Checking ECS Service Status:** The `aws ecs describe-services` command was used to monitor the service's deployment status.
3.  **Checking Target Group Health:** The `aws elbv2 describe-target-health` command confirmed that the Fargate task was healthy and registered with the target group, indicating the application inside the container was running correctly.
4.  **Direct IP Access Attempt:** Attempts to access the task directly via its public IP failed due to security group rules, confirming proper network isolation.
5.  **Final Successful Access:** After DNS propagation, a `curl` request to the ALB's DNS name returned a "405 Method Not Allowed" error, which confirmed that the request successfully reached the application. This indicated the application was online and accessible via the load balancer.

### Step 5: Custom Domain Setup with Route 53

Finally, AWS Route 53 was configured to point the custom domain (`tic-tac-toe.nithish.net`) to the deployed application.

1.  **Identify Hosted Zone ID:** The Hosted Zone ID for the domain `nithish.net` was retrieved from Route 53.
    ```bash
    aws route53 list-hosted-zones --query "HostedZones[?Name == 'nithish.net.']" --region us-east-1
    ```
    The ID was identified as `Z00518181OKPN6XT40XPL`.

2.  **Create Route 53 Record Set JSON:** A JSON file (`route53-record.json`) was created to define an "A" record with an "Alias" target pointing to the Application Load Balancer.
    ```json
    {
      "Changes": [
        {
          "Action": "CREATE",
          "ResourceRecordSet": {
            "Name": "tic-tac-toe.nithish.net",
            "Type": "A",
            "AliasTarget": {
              "HostedZoneId": "Z35SXDOTRQ7X7K", // This is the Hosted Zone ID of the ALB itself
              "DNSName": "tic-tac-toe-lb-813944911.us-east-1.elb.amazonaws.com", // Your ALB's DNS name
              "EvaluateTargetHealth": false
            }
          }
        }
      ]
    }
    ```

3.  **Create the A Record:** The `change-resource-record-sets` command was used to apply this change to the Route 53 hosted zone.
    ```bash
    aws route53 change-resource-record-sets --hosted-zone-id Z00518181OKPN6XT40XPL --change-batch file://route53-record.json --region us-east-1
    ```

Once DNS propagation is complete, your Tic-Tac-Toe application will be accessible via `tic-tac-toe.nithish.net`.
