# Deployment Summary for tic-tac-toe

This document summarizes the AWS resources and configuration used to deploy the tic-tac-toe application.

## Project Details

*   **Project Name:** `tic-tac-toe`
*   **AWS Region:** `us-east-1`
*   **AWS Account ID:** `207888179522`
*   **GitHub Repository:** `https://github.com/nithish-95/tic-tac-toe.git`
*   **Branch:** `Hosted`

## AWS Resources

*   **Amazon ECR Repository URI:** `207888179522.dkr.ecr.us-east-1.amazonaws.com/tic-tac-toe`
*   **Amazon ECS Cluster Name:** `tic-tac-toe`
*   **Amazon ECS Service Name:** `tic-tac-toe-service`
*   **AWS CodePipeline Name:** `tic-tac-toe-pipeline`
*   **AWS CodeBuild Project Name:** `tic-tac-toe-build`
*   **IAM Roles:**
    *   `CodePipelineServiceRole`
    *   `CodeBuildServiceRole`
*   **Amazon S3 Artifact Bucket:** `codepipeline-us-east-1-207888179522`

## Redeployment Steps

To redeploy the application, you will need to recreate these AWS resources. You can use the `.json` files in this repository as templates for creating the CodePipeline and CodeBuild projects.

**Note:** The `codepipeline.json` file contains a hardcoded GitHub OAuth token. This is a security risk. Before redeploying, you should remove the token from the file and configure the pipeline to use a more secure method for authentication, such as AWS CodeStar Connections.
