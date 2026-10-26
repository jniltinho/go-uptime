## MODIFIED Requirements

### Requirement: Formato e validação estrita
The system MUST accept the definition of a managed endpoint in YAML or JSON, with the same keys as an item of `endpoints` in the configuration file, and MUST apply at least the same validations and default values applied to the endpoints of the YAML. In addition, it MUST reject with 400 unknown keys, alerts of a nonexistent type, alerts whose provider is configured neither in the file nor among the valid managed providers, and invalid provider overrides.

#### Scenario: Definição mínima em JSON
- **WHEN** an administrator validates `{"name": "site", "url": "https://exemplo.com", "conditions": ["[STATUS] == 200"]}`
- **THEN** the returned effective definition has an interval of 1 minute and the `GET` method
- **AND** the storable definition does not contain those default values

#### Scenario: Definição sem condições
- **WHEN** an administrator submits a definition without `conditions`
- **THEN** the API answers 400 with the message of the validation error
- **AND** nothing is stored nor monitored

#### Scenario: Chave desconhecida
- **WHEN** an administrator submits a definition with the key `intervall`
- **THEN** the API answers 400 naming the unknown key

#### Scenario: Alerta sem provedor configurado
- **WHEN** neither the configuration nor the web configure the `slack` provider and an administrator creates an endpoint with a `slack` alert
- **THEN** the API answers 400 saying the `slack` provider is not configured

#### Scenario: Alert with a managed provider
- **WHEN** the file has no `telegram` provider, the web has a valid managed `telegram` and an administrator creates an endpoint with a `telegram` alert
- **THEN** the endpoint is created and the alert uses the `default-alert` of the managed `telegram`
