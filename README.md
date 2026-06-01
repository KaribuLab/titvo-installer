# Titvo Installer

Instalador automatizado de **Titvo Security Scan**, la herramienta de análisis de seguridad de código de Titvo.

Este programa se encarga de dejar funcionando toda la plataforma de Titvo dentro de tu cuenta de AWS. Tú solo respondes algunas preguntas (o entregas un archivo de configuración) y el instalador hace el resto: descarga las herramientas necesarias, crea la infraestructura en la nube y deja todo listo para que empieces a usar Titvo.

---

## ¿Qué hace el instalador por ti?

Cuando ejecutas el instalador, el programa realiza automáticamente los siguientes pasos:

1. **Recolecta la configuración** — Te pregunta los datos necesarios (credenciales de AWS, red, claves de IA, etc.) o los lee desde un archivo de configuración que tú le entregas.
2. **Descarga las herramientas** — Baja a tu computador las herramientas que necesita para trabajar (Terraform, Terragrunt y Node.js). No necesitas instalarlas tú: quedan guardadas en una carpeta propia de Titvo (`~/.titvo`).
3. **Despliega la infraestructura en AWS** — Crea en tu cuenta de AWS todos los servicios que Titvo necesita para funcionar (redes, bases de datos, colas de trabajo, contenedores, etc.).
4. **Deja la configuración inicial lista** — Crea tu primer usuario, genera una **API Key** y guarda la configuración de la IA que elegiste.

Al final, el instalador te entrega los datos más importantes en pantalla: tu **User ID**, tu **API Key** y la dirección (endpoint) para usar Titvo.

> **Importante:** La API Key y el User ID se muestran **una sola vez** al finalizar. Guárdalos en un lugar seguro.

---

## Antes de empezar (requisitos)

Para usar el instalador necesitas:

- **Una cuenta de AWS** con permisos para crear infraestructura (redes, bases de datos, contenedores, etc.).
- **Credenciales de AWS** (Access Key ID y Secret Access Key, y opcionalmente un Session Token).
- **Datos de tu red en AWS**:
  - El ID de tu VPC.
  - El rango (CIDR) de una subred privada.
  - La zona de disponibilidad (Availability Zone).
  - El ID de un NAT Gateway.
- **Una clave AES de exactamente 32 caracteres** (se usa para cifrar tus secretos). Debe tener 32 caracteres, ni más ni menos.
- **Una cuenta y clave (API Key) de un proveedor de IA**: Anthropic, OpenAI o Google.
- **Un modelo de embeddings** (por ejemplo `text-embedding-3-small`).
- Credenciales de **Bitbucket** y/o **GitHub**. Debes proporcionar **al menos una** de las dos; también puedes configurar ambas.
- **Conexión a internet** (el instalador descarga herramientas y código desde internet).

> No necesitas tener instalados Terraform, Terragrunt ni Node.js previamente. El instalador los descarga por ti.

---

## Cómo ejecutar el instalador

### Paso 1: Obtener el ejecutable

Puedes compilar el instalador desde el código fuente. Necesitas tener instalado [Go](https://go.dev/) y [Task](https://taskfile.dev/).

```bash
task titvo:build
```

Esto genera el ejecutable en `./bin/titvo`.

### Paso 2: Ejecutar la instalación

Hay dos formas de ejecutar el instalador.

#### Opción A: Modo interactivo (recomendado para la primera vez)

El instalador te irá preguntando cada dato paso a paso:

```bash
./bin/titvo
```

Responde cada pregunta a medida que aparezca. Para los datos sensibles (como claves y tokens) el texto no se mostrará en pantalla mientras lo escribes.

#### Opción B: Usando un archivo de configuración

Si prefieres no responder pregunta por pregunta, puedes crear un archivo `config.json` con todos los datos y pasárselo al instalador:

```bash
./bin/titvo --config config.json
```

El archivo `config.json` tiene esta forma:

```json
{
  "aws_access_key_id": "TU_ACCESS_KEY",
  "aws_secret_access_key": "TU_SECRET_KEY",
  "aws_session_token": "",
  "aws_region": "us-east-1",
  "vpc_id": "vpc-xxxxxxxxxxxxxxxxx",
  "private_subnet_cidr": "172.31.64.0/20",
  "availability_zone": "us-east-1a",
  "nat_gateway_id": "nat-xxxxxxxxxxxxxxxxx",
  "aes_secret": "una-clave-de-exactamente-32-chars",
  "user_name": "Tu Nombre",
  "ai_provider": "openai",
  "ai_model": "gpt-4o",
  "ai_api_key": "TU_API_KEY_DE_IA",
  "embedding_provider": "",
  "embedding_model": "text-embedding-3-small",
  "embedding_api_key": "",
  "github_access_token": "",
  "bitbucket_api_token": ""
}
```

> **Nunca compartas ni subas tu `config.json` a un repositorio.** Contiene credenciales y claves sensibles.

Notas sobre algunos campos:

- `aes_secret` debe tener **exactamente 32 caracteres**.
- `embedding_model` es **obligatorio**.
- Si dejas `embedding_provider` o `embedding_api_key` vacíos, se usarán los mismos valores que configuraste para la IA.
- Debes proporcionar **al menos uno** de `github_access_token` o `bitbucket_api_token` (también puedes incluir ambos). Si dejas ambos vacíos, la instalación se detendrá con un error.

### Modo depuración (opcional)

Si necesitas ver más detalle de lo que ocurre durante la instalación, agrega la opción `--debug`:

```bash
./bin/titvo --debug
```

---

## ¿Qué verás durante la instalación?

El instalador muestra mensajes a medida que avanza:

- Mensajes **verdes**: información del progreso (qué está haciendo en cada momento).
- Mensajes **amarillos**: advertencias o preguntas (por ejemplo, cuando una integración opcional se omite).
- Mensajes **rojos**: errores. Si aparece un error, la instalación se detiene.

La instalación puede **tardar varios minutos**, ya que descarga herramientas y crea recursos reales en AWS. Es normal que veas mucha actividad en pantalla.

---

## Al finalizar

Cuando la instalación termina con éxito, verás un resumen como este:

```
----------------------------------------------------------------
- Setup Endpoint: https://...
- User ID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
- API Key: tvok-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
----------------------------------------------------------------
* Recuerda guardar tu API Key y User ID en un lugar seguro
----------------------------------------------------------------
```

Guarda estos datos en un lugar seguro: el **User ID** y la **API Key** se muestran una sola vez y los necesitarás para usar Titvo.

---

## Comandos adicionales

Una vez que Titvo ya está instalado, el ejecutable ofrece dos comandos extra para administrar la configuración:

### Agregar un secreto (cifrado)

Guarda un valor sensible en la configuración de Titvo. El valor se cifra automáticamente antes de guardarse.

```bash
./bin/titvo secret
```

### Agregar un parámetro (sin cifrar)

Guarda un valor de configuración en texto plano.

```bash
./bin/titvo parameter
```

Ambos comandos te pedirán las credenciales de AWS (o puedes pasarlas con `--config`) y verificarán que Titvo ya esté instalado antes de continuar.

---

## ¿Algo salió mal?

- **"AES Secret must have 32 characters in length"**: tu clave AES no tiene exactamente 32 caracteres. Corrígela y vuelve a intentar.
- **"embedding_model is required"**: falta el campo del modelo de embeddings en tu configuración.
- **Errores de credenciales de AWS**: revisa que tu Access Key, Secret Key y región sean correctos y que tengan los permisos necesarios.
- La instalación se detiene ante el primer error. Corrige el problema indicado en el mensaje rojo y vuelve a ejecutar el instalador.

---

## Repositorios de Titvo utilizados

Durante la instalación, el instalador clona y despliega varios repositorios de la organización [KaribuLab](https://github.com/KaribuLab). A continuación se listan todos los componentes que se despliegan:

### Infraestructura base

| Repositorio | Descripción |
|-------------|-------------|
| [titvo-security-scan-infra-aws](https://github.com/KaribuLab/titvo-security-scan-infra-aws) | Infraestructura base de Titvo en AWS (red privada, recursos compartidos, parámetros y secretos). Es lo primero que se despliega. |
| [titvo-installer-ecr-publisher](https://github.com/KaribuLab/titvo-installer-ecr-publisher) | Componente temporal que publica las imágenes de contenedor en ECR mediante jobs de AWS Batch. Se despliega, ejecuta los jobs y luego se destruye. |

### Servicios principales (contenedores)

| Repositorio | Descripción |
|-------------|-------------|
| [titvo-agent-aws](https://github.com/KaribuLab/titvo-agent-aws) | Agente que ejecuta el análisis de seguridad del código. Incluye su repositorio ECR y la infraestructura para correr el contenedor. |
| [titvo-mcp-gateway](https://github.com/KaribuLab/titvo-mcp-gateway) | Gateway MCP (Model Context Protocol) que expone las herramientas que utiliza el agente. Incluye su imagen ECR e infraestructura. |
| [titvo-rag-indexer](https://github.com/KaribuLab/titvo-rag-indexer) | Indexador RAG encargado de generar y mantener los embeddings/índices del código. Incluye su imagen ECR e infraestructura. |

### Componentes de tareas y autenticación

| Repositorio | Descripción |
|-------------|-------------|
| [titvo-auth-setup-aws](https://github.com/KaribuLab/titvo-auth-setup-aws) | Configuración de autenticación de la plataforma. |
| [titvo-task-trigger-aws](https://github.com/KaribuLab/titvo-task-trigger-aws) | Disparador de tareas: recibe las solicitudes y da inicio a los escaneos. |
| [titvo-task-status-aws](https://github.com/KaribuLab/titvo-task-status-aws) | Consulta del estado y resultado de las tareas de análisis. |
| [titvo-task-cli-files-aws](https://github.com/KaribuLab/titvo-task-cli-files-aws) | Manejo de los archivos asociados a las tareas ejecutadas desde la CLI. |
| [titvo-git-commit-files-aws](https://github.com/KaribuLab/titvo-git-commit-files-aws) | Obtiene los archivos modificados en un commit de Git para su análisis. |

### Componentes de reporte e integraciones

| Repositorio | Descripción |
|-------------|-------------|
| [titvo-issue-report-aws](https://github.com/KaribuLab/titvo-issue-report-aws) | Generación de reportes de los hallazgos del análisis. |
| [titvo-bitbucket-code-insights-aws](https://github.com/KaribuLab/titvo-bitbucket-code-insights-aws) | Integración con Bitbucket Code Insights. Solo se despliega si configuras credenciales de Bitbucket. |
| [titvo-github-issue-aws](https://github.com/KaribuLab/titvo-github-issue-aws) | Integración con GitHub Issues. Solo se despliega si configuras credenciales de GitHub. |

> Además, el instalador descarga herramientas de terceros (Terraform, Terragrunt y Node.js) que no son repositorios de Titvo.

---

## Documentación técnica

Si quieres entender en detalle cómo funciona el instalador por dentro (flujo paso a paso, diagramas de secuencia y arquitectura), revisa la carpeta [`docs/`](./docs):

- [`docs/arquitectura.md`](./docs/arquitectura.md) — Arquitectura del proyecto.
- [`docs/diagramas-secuencia.md`](./docs/diagramas-secuencia.md) — Diagramas de secuencia de cada paso de la instalación.
