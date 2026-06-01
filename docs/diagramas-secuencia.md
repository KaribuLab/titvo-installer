# Diagramas de secuencia

Este documento describe, paso a paso, el flujo de ejecución del instalador definido en [`internal/installer.go`](../internal/installer.go), función `RunInstaller`.

`RunInstaller` orquesta cuatro pasos en orden estricto. Si alguno falla, el proceso se detiene con `printErrorAndExit`:

1. **Setup** — obtención de la configuración.
2. **Install Tools** — descarga de herramientas.
3. **Deploy Infra** — despliegue de la infraestructura en AWS.
4. **Start Configuration** — registro de la configuración inicial.

Cada diagrama indica el **archivo Go** y las **funciones** involucradas.

---

## Diagrama general (orquestación)

Vista de alto nivel del flujo completo en `RunInstaller`.

```mermaid
sequenceDiagram
    autonumber
    actor User as Usuario
    participant Main as main.go<br/>RunInstaller (installer.go)
    participant Setup as setup.go
    participant Tool as tool.go<br/>InstallTools
    participant Deploy as deploy_runtime.go<br/>deployInfra
    participant Start as start.go<br/>StartConfiguration

    User->>Main: ejecuta titvo-installer [--config] [--debug]
    Main->>Main: lee flags debug y config

    alt --config provisto
        Main->>Main: os.ReadFile + json.Unmarshal (SetupConfigFile)
        Main->>Main: valida aes_secret (32), embedding_model<br/>y al menos 1 de Bitbucket/GitHub
    else modo interactivo
        Main->>Setup: SetupInstallation()
        Setup-->>Main: *SetupConfig
    end

    Main->>Tool: InstallTools()
    Tool-->>Main: *InstallToolConfig

    Main->>Setup: setup.AWSCredentialsLookup.GetCredentials()
    Setup-->>Main: *AWSCredentials

    Main->>Deploy: DeployInfra(DeployConfig)
    Deploy-->>Main: error / nil

    Main->>Start: StartConfiguration(StartConfig)
    Start-->>Main: error / nil
    Main-->>User: User ID, API Key y endpoint
```

**Explicación:**

`RunInstaller` ([`installer.go`](../internal/installer.go)) primero obtiene los flags `debug` y `config` desde Cobra. Si se entregó un archivo de configuración, lo lee con `os.ReadFile`, lo deserializa a `SetupConfigFile` y valida tres reglas críticas: el `aes_secret` debe tener 32 caracteres, `embedding_model` no puede estar vacío y debe existir al menos uno de `bitbucket_api_token` o `github_access_token`. Si no hay archivo, delega en `SetupInstallation` para el flujo interactivo. Luego ejecuta secuencialmente `InstallTools`, obtiene las credenciales AWS mediante la interfaz `AWSCredentialsLookup`, despliega la infraestructura con `DeployInfra` y finaliza con `StartConfiguration`, que imprime el resumen final.

---

## Paso 1 — Setup (`setup.go`, `prompt.go`)

Obtención de la configuración. Solo aplica el flujo interactivo cuando **no** se pasa `--config`.

```mermaid
sequenceDiagram
    autonumber
    participant Main as installer.go<br/>RunInstaller
    participant Setup as setup.go<br/>SetupInstallation
    participant Prompt as prompt.go
    participant FileCreds as setup.go<br/>AWSFileCredentials

    Main->>Setup: SetupInstallation()
    Setup->>Prompt: askForInput("AWS Region")
    Setup->>Prompt: askForChoices("Input o File?")

    alt Opción "Input" (1)
        Prompt->>Setup: askForPromptInput(awsRegion)
        Setup->>Prompt: askForPassword(AWS keys, AES, API keys...)
        Setup->>Prompt: askForInput(VPC, subnet, AZ, NAT, userName, modelos)
        Setup->>Prompt: askForAIProvider() / askForEmbeddingProvider()
        Setup->>Prompt: askForYesNo(embedding/bitbucket/github)
        Note over Setup: valida len(aesSecret) == 32<br/>valida al menos 1 de Bitbucket/GitHub
        Setup-->>Main: *SetupConfig (InputCredential)
    else Opción "File" (2)
        Prompt->>Setup: askForCredentialsFile(awsRegion)
        Note over Setup: usa AWSFileCredentials (perfil ~/.aws/credentials)<br/>valida al menos 1 de Bitbucket/GitHub
        Setup-->>Main: *SetupConfig (AWSFileCredentials)
    end
```

**Explicación:**

`SetupInstallation` ([`setup.go`](../internal/setup.go)) pregunta primero la región de AWS y luego ofrece dos formas de entregar credenciales mediante `askForChoices`:

- **Input** (`askForPromptInput`): el usuario escribe todos los datos directamente. Los valores sensibles (claves de AWS, AES, API keys, tokens) se leen con `askForPassword` (entrada oculta). Aquí se valida que el `aes_secret` tenga exactamente 32 caracteres. El proveedor de IA y de embeddings se eligen con `askForAIProvider` y `askForEmbeddingProvider`. La integración de Embeddings es opcional, pero se debe proporcionar **al menos una** credencial de Bitbucket o GitHub (pueden ser ambas); si no se entrega ninguna, el proceso se detiene con un error. Devuelve un `SetupConfig` con un `InputCredential`.
- **File** (`askForCredentialsFile` en [`prompt.go`](../internal/prompt.go)): el usuario indica un perfil de AWS; las credenciales se leerán del archivo `~/.aws/credentials` mediante `AWSFileCredentials`. También aquí se exige al menos una credencial de Bitbucket o GitHub.

> Cuando se usa `--config`, este paso se omite: `RunInstaller` construye el `SetupConfig` directamente desde el JSON usando un `SetupConfigFileLookup`.

---

## Paso 2 — Install Tools (`tool.go`, `util.go`, `os.go`)

Descarga e instalación de Terragrunt, Terraform y Node.js en `~/.titvo`.

```mermaid
sequenceDiagram
    autonumber
    participant Main as installer.go<br/>RunInstaller
    participant Tool as tool.go<br/>InstallTools
    participant OS as os.go
    participant Util as util.go
    participant Net as Releases públicos

    Main->>Tool: InstallTools()
    Tool->>Tool: os.UserHomeDir() + MkdirAll(~/.titvo/bin)
    Tool->>OS: GetOS() / GetArch()

    Tool->>Tool: DownloadTerragrunt(binDir, "0.69.1", os, arch)
    Tool->>Util: downloadFile(terragruntUrl)
    Tool->>OS: ExecuteWithOptions(terragrunt --version)

    Tool->>Tool: DownloadTerraform(binDir, "1.9.8", os, arch)
    Tool->>Util: downloadFile(terraformUrl) + extractZip()
    Tool->>OS: ExecuteWithOptions(terraform --version)

    Tool->>Tool: DownloadNode(titvoDir, "20.19.4", os, arch)
    Tool->>Util: downloadFile(nodeUrl) + extractTarGz()
    Tool->>OS: ExecuteWithOptions(node --version / npm --version)

    Tool->>Tool: LogConfiguredToolBinaries(config)
    Tool-->>Main: *InstallToolConfig
```

**Explicación:**

`InstallTools` ([`tool.go`](../internal/tool.go)) resuelve el directorio *home* del usuario y crea `~/.titvo/bin`. Detecta el sistema operativo y la arquitectura con `GetOS`/`GetArch` ([`os.go`](../internal/os.go)) para elegir las URLs de descarga correctas. Luego descarga, en orden, tres herramientas con versiones fijas:

- **Terragrunt** `0.69.1` (`DownloadTerragrunt`): se descarga el binario con `downloadFile` y, en sistemas no Windows, se le da permiso de ejecución (`os.Chmod`).
- **Terraform** `1.9.8` (`DownloadTerraform`): se descarga un ZIP y se descomprime con `extractZip` ([`util.go`](../internal/util.go)); el ZIP se elimina al final.
- **Node.js** `20.19.4` (`DownloadNode`): se descarga un `tar.gz` y se descomprime con `extractTarGz`.

Tras cada descarga se valida el binario ejecutando `--version` con `ExecuteWithOptions`. Finalmente, `LogConfiguredToolBinaries` verifica que los cuatro binarios (Terragrunt, Terraform, Node, npm) existan y funcionen, y advierte si en el `PATH` del sistema hay versiones distintas. Devuelve un `InstallToolConfig` con todas las rutas.

---

## Paso 2.5 — Obtención de credenciales AWS (`setup.go`)

Entre el paso 2 y el 3, `RunInstaller` resuelve las credenciales AWS.

```mermaid
sequenceDiagram
    autonumber
    participant Main as installer.go<br/>RunInstaller
    participant Lookup as AWSCredentialsLookup<br/>(setup.go)

    Main->>Lookup: setup.AWSCredentialsLookup.GetCredentials()

    alt SetupConfigFileLookup (--config)
        Lookup-->>Main: AWSCredentials desde el JSON
    else InputCredential (input interactivo)
        Lookup-->>Main: AWSCredentials ingresadas
    else AWSFileCredentials (perfil)
        Lookup->>Lookup: lee ~/.aws/credentials (ini.Load)
        Lookup-->>Main: AWSCredentials del perfil
    end
```

**Explicación:**

`AWSCredentialsLookup` ([`setup.go`](../internal/setup.go)) es una interfaz con tres implementaciones. Según cómo se obtuvo la configuración, `GetCredentials()` retorna las credenciales desde el JSON (`SetupConfigFileLookup`), desde los valores ingresados interactivamente (`InputCredential`) o leyendo el archivo `~/.aws/credentials` con el parser INI (`AWSFileCredentials`). Este patrón desacopla el origen de las credenciales del resto del flujo.

---

## Paso 3 — Deploy Infra (`deploy.go`, `deploy_runtime.go`, `aws.go`, `os.go`)

Despliegue de toda la infraestructura. Es el paso más extenso.

```mermaid
sequenceDiagram
    autonumber
    participant Main as installer.go<br/>RunInstaller
    participant Deploy as deploy_runtime.go<br/>deployInfra
    participant Src as deploy.go<br/>Download*Source
    participant AWS as aws.go
    participant OS as os.go (Terragrunt/npm)

    Main->>Deploy: DeployInfra(config) → deployInfra(config)
    Deploy->>Deploy: MkdirAll(~/.titvo/infra)
    Deploy->>Src: DownloadInfraSource() (git clone)
    Deploy->>AWS: GetAccountID() (STS)
    Deploy->>Deploy: arma env (claves AWS, PATH, TG_TF_PATH...)

    Note over Deploy,AWS: Parámetros base
    Deploy->>AWS: PutParameter(vpc-id, private-subnets)
    Deploy->>AWS: CreateSecret(aes_secret) (Secrets Manager)
    Deploy->>AWS: PutParameter(encryption-key-name/arn)

    Deploy->>OS: runTerragrunt(base infra, apply)

    opt Bitbucket / GitHub configurados
        Deploy->>Deploy: encrypt(token) (util.go)
        Deploy->>AWS: PutRecord(scm token) (DynamoDB)
    end

    Note over Deploy,OS: Componentes Node (build + apply)
    Deploy->>Src: Download git-commit-files
    Deploy->>OS: npm install + npm run build + terragrunt apply

    Note over Deploy,OS: Imágenes ECR
    Deploy->>Src: Download agent / mcp-gateway / rag-indexer
    Deploy->>OS: terragrunt apply (ecr de cada uno)

    Note over Deploy,AWS: Publicación de imágenes
    Deploy->>Src: Download installer-ecr-publisher
    Deploy->>OS: terragrunt apply (publisher)
    Deploy->>AWS: SubmitBatchJob(agent / mcp / rag) (AWS Batch)
    Deploy->>OS: terragrunt destroy (publisher)

    Note over Deploy,OS: Reporte + core
    Deploy->>OS: deploy issue-report (+ bitbucket/github según config)
    Deploy->>OS: apply rag-indexer / agent / mcp-gateway
    Deploy->>OS: deploy auth-setup, task-cli-files, task-trigger, task-status
    Deploy-->>Main: nil (Deployed all services)
```

**Explicación:**

`DeployInfra` ([`deploy.go`](../internal/deploy.go)) delega en `deployInfra` ([`deploy_runtime.go`](../internal/deploy_runtime.go)). El flujo es:

1. **Preparación**: crea `~/.titvo/infra`, clona el repositorio base con `DownloadInfraSource` (un `git clone` vía `downloadSource`), obtiene el Account ID con `GetAccountID` ([`aws.go`](../internal/aws.go)) y arma el mapa de variables de entorno que recibirán Terragrunt y npm (credenciales AWS, `PATH` con los binarios descargados, `TG_TF_PATH`, caché de plugins, etc.). Con `--debug` se activan `TG_LOG` y `TF_LOG`.
2. **Parámetros base**: escribe `vpc-id` y la configuración de subred privada en SSM con `PutParameter`, crea el secreto `aes_secret` (codificado en base64) en Secrets Manager con `CreateSecret`, y guarda en SSM el nombre y ARN de la clave de cifrado.
3. **Infra base**: ejecuta `terragrunt run-all apply` sobre `prod/us-east-1` con `runTerragrunt` (que internamente usa `ExecuteWithOptions` de [`os.go`](../internal/os.go)).
4. **Secretos SCM (opcional)**: si se entregaron tokens de Bitbucket o GitHub, los cifra con `encrypt` ([`util.go`](../internal/util.go)) y los guarda en DynamoDB con `PutRecord`. Si faltan, se omite la integración con una advertencia.
5. **Componentes Node**: para componentes como `git-commit-files`, actualiza submódulos, ejecuta `npm install` + `npm run build` y luego `terragrunt apply` (`deployNodeComponent`).
6. **Imágenes ECR**: clona `agent`, `mcp-gateway` y `rag-indexer`, y aplica el módulo Terragrunt de su carpeta `aws/ecr` para crear los repositorios ECR.
7. **Publicación de imágenes**: despliega `installer-ecr-publisher`, lee de SSM el ARN de la job definition y job queue, lanza jobs de **AWS Batch** con `SubmitBatchJob` para publicar las imágenes (agente, MCP gateway, RAG indexer) y luego destruye el publisher con `terragrunt destroy`.
8. **Componentes de reporte y core**: despliega `issue-report` (y `bitbucket-code-insights`/`github-issue` según las integraciones activas), aplica las capas de `rag-indexer`, `agent` y `mcp-gateway`, y finalmente los componentes core: `auth-setup`, `task-cli-files`, `task-trigger` y `task-status`.

> **Nota sobre pruebas:** las llamadas a comandos externos y a AWS se realizan a través de variables de función (`executeWithOptionsFn`, `getAccountIDFn`, `putParameterFn`, `submitBatchJobFn`, etc.) definidas en `deploy_runtime.go`, lo que permite sustituirlas por *mocks* en los tests.

---

## Paso 4 — Start Configuration (`start.go`, `aws.go`, `util.go`)

Registro de la configuración inicial: usuario, API key y parámetros.

```mermaid
sequenceDiagram
    autonumber
    participant Main as installer.go<br/>RunInstaller
    participant Start as start.go<br/>StartConfiguration
    participant AWS as aws.go
    participant Util as util.go

    Main->>Start: StartConfiguration(StartConfig)

    Start->>AWS: GetParameter(user-table-name)
    Start->>AWS: PutRecord(user: user_id, account_type=Team, name)

    Start->>AWS: GetParameter(apikey-table-name)
    Start->>Start: generateAPIKey() + hashSha256()
    Start->>AWS: PutRecord(api_key hasheada)

    Start->>AWS: GetParameter(parameter-table-name)
    Start->>AWS: PutRecord(ai_provider, ai_model)

    Start->>AWS: GetParameter(cli-files bucket) → PutRecord
    Note over Start: valida len(AESSecret) == 32
    Start->>Util: encrypt(ai_api_key, AESSecret)
    Start->>AWS: PutRecord(ai_api_key cifrada)

    Start->>AWS: GetParameter(job_queue / task_endpoint / job_definition)
    Start->>AWS: PutRecord(varios parámetros + mcp_server_url)

    Start->>AWS: GetParameter(rag-index bucket) → PutRecord
    Start->>Util: encrypt(embedding_api_key, AESSecret)
    Start->>AWS: PutRecord(embedding_provider/model/api_key)

    Start->>AWS: GetParameter(setup endpoint)
    Start-->>Main: imprime Setup Endpoint, User ID y API Key
```

**Explicación:**

`StartConfiguration` ([`start.go`](../internal/start.go)) deja lista la configuración funcional de Titvo en DynamoDB. Su patrón general es: leer de SSM (`GetParameter`) los nombres de las tablas y recursos creados por la infraestructura, y luego insertar registros con `PutRecord` ([`aws.go`](../internal/aws.go)):

1. **Usuario**: lee el nombre de la tabla de usuarios y crea un registro con un `user_id` (UUID), `account_type = Team` y el nombre indicado.
2. **API Key**: genera una clave aleatoria con `generateAPIKey` (prefijo `tvok-`, 48 caracteres), la **hashea con SHA-256** (`hashSha256`) y guarda solo el hash. El valor en claro solo se mostrará al usuario al final.
3. **Parámetros de IA**: guarda `ai_provider` y `ai_model`. La `ai_api_key` se **cifra con AES** (`encrypt` de [`util.go`](../internal/util.go), que exige clave de 32 caracteres) antes de guardarse.
4. **Parámetros de infraestructura**: guarda el bucket de archivos de la CLI, la cola y definición de jobs de Batch, los endpoints de la API y la URL del MCP server.
5. **Parámetros de embeddings**: si `embedding_provider` o `embedding_api_key` están vacíos, usa como respaldo los valores de IA. El `embedding_model` se guarda en texto plano y la API key de embeddings se cifra.
6. **Resumen final**: imprime el Setup Endpoint, el `User ID` y la `API Key` en claro, recordando al usuario guardarlos en un lugar seguro (se muestran una sola vez).

---

## Flujos auxiliares — Subcomandos `secret` y `parameter` (`parameter_loader.go`)

No forman parte de `RunInstaller`, pero son parte del mismo binario y se usan después de instalar.

```mermaid
sequenceDiagram
    autonumber
    actor User as Usuario
    participant Main as main.go
    participant PL as parameter_loader.go
    participant AWS as aws.go
    participant Util as util.go

    User->>Main: titvo-installer secret | parameter [--config]
    Main->>PL: RunSecretLoader / RunParameterLoader
    PL->>PL: prepareCredentials() (archivo o input)
    PL->>AWS: GetSecret(aes_secret) (verifica instalación)

    alt secret (cifrado)
        PL->>AWS: GetSecret(aes_secret) + base64 decode
        loop por cada secreto
            PL->>Util: encrypt(valor, aesSecret)
            PL->>AWS: PutRecord(parameter_id, valor cifrado)
        end
    else parameter (texto plano)
        loop por cada parámetro
            PL->>AWS: PutRecord(parameter_id, valor)
        end
    end
```

**Explicación:**

Ambos comandos ([`parameter_loader.go`](../internal/parameter_loader.go)) comparten `prepareCredentials`, que obtiene las credenciales AWS (desde `--config` o por input) y verifica que Titvo esté instalado intentando leer el secreto `aes_secret` con `GetSecret`. Si no existe, aborta indicando que primero hay que ejecutar el instalador.

- `RunSecretLoader`: obtiene la clave AES desde Secrets Manager, la decodifica de base64, y en un bucle pide pares nombre/valor, **cifra** cada valor con `encrypt` y lo guarda en la tabla `tvo-security-scan-parameter-prod` con `PutRecord`.
- `RunParameterLoader`: similar, pero guarda los valores **en texto plano** (sin cifrar).
