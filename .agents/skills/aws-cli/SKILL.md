---
name: aws-cli
description: Use AWS CLI with dotenv credentials
---

# aws-cli

1. Install AWS CLI v2
2. Create an .env file on the root of this project.

## When to use

If you need get information from AWS services. 

## Instructions

1. Load AWS credentials stored on `.env` placed on the root of this project. 
```shell
set -a && source .env && set +a
```
2. Then you can execute al AWS CLI v2 supported commands

## Example

```shell
aws s3 ls
```

## References

- AWS CLI v2 documentation: https://aws.amazon.com/cli/
- AWS CLI v2 examples: https://aws.amazon.com/cli/user-guide/examples/
- AWS CLI v2 configuration: https://aws.amazon.com/cli/user-guide/cli-configure-files/
- AWS CLI v2 configuration: https://aws.amazon.com/cli/user-guide/cli-configure-files/  