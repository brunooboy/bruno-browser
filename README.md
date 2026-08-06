![Bruno Browser](docs/assets/bruno-browser-banner.png)

# Bruno Browser

Navegador de perfis local-first para Windows, feito com Go, Wails, React e TypeScript. Cada perfil usa um diretório físico próprio do Chromium, preservando cookies, sessões e armazenamento local no disco.

> Versão atual: **0.8.0** · Windows 10/11 · x64 e ARM64

## Instalação

1. Baixe `Bruno Browser Setup.exe` na página de Releases.
2. Abra o instalador e conclua as etapas exibidas.
3. Inicie o Bruno Browser pelo Menu Iniciar ou pelo atalho da área de trabalho.

O instalador identifica automaticamente se o Windows é x64 ou ARM64, instala somente o executável correto em `%LocalAppData%\Programs\Bruno Browser` e não exige privilégios de administrador. Se o Microsoft WebView2 Runtime não estiver disponível, o instalador obtém e instala o componente oficial apropriado.

O pacote atual ainda não possui assinatura Authenticode. Por isso, o Microsoft SmartScreen pode exibir um aviso em algumas máquinas. Para eliminar esse aviso em distribuição pública é necessário assinar o instalador e o executável com um certificado de assinatura de código confiável.

## Funcionalidades

- Perfis persistentes com `--user-data-dir` exclusivo e restauração de sessão.
- Metadados, plataformas, tags, notas e maturidade por perfil.
- Inicialização do Chromium/Wayfern em janela separada e identificada pelo perfil.
- Fingerprint persistente por perfil aplicado por CDP quando o navegador permite automação.
- Proxy HTTP/SOCKS5 por perfil, DNS pelo proxy e proteção contra vazamento WebRTC.
- Cofre de extensões CRX com associação posterior a um ou vários perfis.
- Limpeza seletiva de cache/histórico, cookies/sessão e exclusão de perfil.
- Login Discord OAuth2 com callback local.
- Licenças locais AES-256-GCM, planos de 1, 7 e 30 dias ou vitalício.
- Changelog e verificação de atualização por manifesto JSON.
- Tema e central de notificações persistentes.

As operações premium são validadas novamente pelo backend antes de iniciar perfil, alterar rede ou gerenciar extensões. Remover uma key ou atingir a data de expiração bloqueia essas operações imediatamente.

## Dados locais

Os dados do usuário ficam em `%AppData%\bruno-browser` e não são removidos na desinstalação:

```text
bruno-browser/
├── account.json
├── appconfig.json
├── extensions/
├── keys-history.json
├── license.json
├── preferences.json
└── profiles/
    └── <profile-uuid>/
        ├── metadata.json
        ├── fingerprint.json
        ├── network.json
        └── chromium/          user-data-dir do navegador
```

O diretório pode ser alterado com `BRUNO_BROWSER_DATA_DIR`. O executável do navegador pode ser escolhido com `BRUNO_BROWSER_EXECUTABLE`.

## Configuração do Discord

1. Crie uma aplicação no [Discord Developer Portal](https://discord.com/developers/applications).
2. Cadastre exatamente `http://localhost:34115/callback` como Redirect URI em OAuth2.
3. Copie `config.example.json` para `config.json` e preencha Client ID, Client Secret e Discord ID do administrador.
4. Execute o app uma vez. A configuração privada será copiada para `%AppData%\bruno-browser\appconfig.json`.

Também são aceitas as variáveis `BRUNO_BROWSER_DISCORD_CLIENT_ID`, `BRUNO_BROWSER_DISCORD_CLIENT_SECRET`, `BRUNO_BROWSER_ADMIN_DISCORD_ID` e `BRUNO_BROWSER_UPDATE_URL`. Variáveis de ambiente têm prioridade sobre o arquivo.

`config.json` e `appconfig.json` são ignorados pelo Git. Nunca publique o Client Secret. O access token do Discord é usado somente para obter os dados do usuário e não é persistido. Depois do primeiro login, conta e licença continuam disponíveis offline.

> Importante: uma instalação distribuída não deve conter o Client Secret dentro do repositório ou do instalador público. Para distribuir o login OAuth a terceiros com segurança, use um fluxo público compatível com PKCE ou um serviço mínimo que mantenha o segredo fora do aplicativo.

## Extensões CRX

O botão **Instalar CRX** abre o seletor nativo. O backend valida CRX2/CRX3, extrai o conteúdo com proteção contra caminhos maliciosos e confere o `manifest.json`. A extensão entra primeiro no cofre global; depois o usuário escolhe em quais perfis ela será carregada. Alterações passam a valer na próxima abertura do perfil.

## Atualizações

`internal/updates/version.json` é incorporado ao executável. O `version.json` da raiz pode ser publicado no GitHub e configurado em `updateUrl`, por exemplo:

```text
https://raw.githubusercontent.com/SEU_USUARIO/bruno-browser/main/version.json
```

Sem uma URL configurada, a página de Atualizações continua exibindo o changelog local.

## Desenvolvimento e testes

Requisitos: Go 1.26+, Node.js 22.12+, npm 10+ e Windows 10/11.

Para executar como aplicativo desktop real:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run-app.ps1
```

Para validar backend e frontend:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\test-app.ps1
```

Para gerar o pacote universal:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-app.ps1
```

Arquivos gerados em `build\bin`:

- `Bruno Browser Setup.exe`: instalador único para Windows x64 e ARM64.
- `Bruno Browser.exe`: versão portátil x64.
- `Bruno Browser-amd64.exe`: binário x64.
- `Bruno Browser-arm64.exe`: binário ARM64.

Para compilar somente um executável portátil, use `-Direct`. Para pular o instalador, use `-NoInstaller`.

## Limites de segurança do licenciamento local

AES-GCM protege a integridade e a confidencialidade contra edição casual. Entretanto, um licenciamento totalmente offline com segredo simétrico dentro do executável não impede um atacante determinado de extrair o segredo e forjar keys. Licenciamento resistente a adulteração exige assinatura assimétrica, mantendo a chave privada fora do aplicativo, ou validação por servidor.

## Licença

Uso privado. Defina uma licença antes de distribuir ou aceitar contribuições públicas.
