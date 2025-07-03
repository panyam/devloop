# Frontend Framework Development Example

This example demonstrates using `devloop` for modern frontend development with multiple frameworks, build tools, testing, and development servers running concurrently.

## What's Included

- **React Application**: Main app with TypeScript and modern tooling
- **Vue.js Components**: Shared component library
- **Storybook**: Component documentation and testing
- **Vite Build System**: Fast development and building
- **Jest Testing**: Unit and integration tests
- **ESLint/Prettier**: Code quality and formatting
- **Sass/PostCSS**: Advanced styling pipeline

## Prerequisites

- Node.js 18+ and npm/yarn
- devloop installed (`go install github.com/panyam/devloop@latest`)

## Quick Start

1. **Install dependencies:**
   ```bash
   make deps
   ```

2. **Start development environment:**
   ```bash
   make run
   # Or directly: devloop -c .devloop.yaml
   ```

3. **Access applications:**
   - React App: http://localhost:3000
   - Storybook: http://localhost:6006
   - Vue Components: http://localhost:3001
   - Test Results: Watch mode in terminal

## Architecture

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│    React    │    │    Vue.js   │    │  Storybook  │
│   Main App  │    │ Components  │    │    Docs     │
│   :3000     │    │   :3001     │    │   :6006     │
└─────────────┘    └─────────────┘    └─────────────┘
        │                 │                 │
        └─────────────────┼─────────────────┘
                          │
                ┌─────────────┐
                │  Shared     │
                │  Assets &   │
                │  Utils      │
                └─────────────┘
```

## Development Workflow

### 1. Hot Reloading
- **React/Vue changes**: Instant browser updates
- **Sass/CSS changes**: Live style injection
- **TypeScript changes**: Fast transpilation and reload
- **Component changes**: Storybook auto-updates

### 2. Testing Pipeline
- **Unit tests**: Run on file changes
- **Integration tests**: Triggered by component updates
- **E2E tests**: Manual trigger or CI integration
- **Type checking**: Continuous TypeScript validation

### 3. Code Quality
- **ESLint**: Automatic linting on save
- **Prettier**: Code formatting on file changes
- **Husky hooks**: Pre-commit quality checks
- **Import sorting**: Automatic import organization

## Try It Out

1. **Modify React components** (`react-app/src/components/`):
   - Change a component
   - See instant hot reload in browser
   - Watch Storybook update automatically

2. **Update Vue components** (`vue-components/src/`):
   - Modify a shared component
   - See updates in both Vue app and Storybook
   - TypeScript errors appear immediately

3. **Change styles** (`shared/styles/`):
   - Update Sass variables
   - Watch live CSS injection
   - See changes across all apps

4. **Write tests** (`__tests__/` directories):
   - Add new test files
   - See test runner execute automatically
   - Watch coverage reports update

## Project Structure

```
05-frontend-framework/
├── .devloop.yaml           # Devloop configuration
├── package.json            # Root package.json for workspaces
├── Makefile               # Build automation
├── react-app/             # Main React application
│   ├── src/
│   │   ├── components/
│   │   ├── pages/
│   │   ├── hooks/
│   │   └── utils/
│   ├── public/
│   ├── package.json
│   └── vite.config.ts
├── vue-components/        # Vue.js component library
│   ├── src/
│   │   ├── components/
│   │   └── composables/
│   ├── package.json
│   └── vite.config.ts
├── storybook/            # Component documentation
│   ├── .storybook/
│   ├── stories/
│   └── package.json
├── shared/               # Shared utilities and assets
│   ├── styles/
│   ├── utils/
│   ├── types/
│   └── package.json
├── tools/                # Build and development tools
│   ├── eslint/
│   ├── prettier/
│   └── webpack/
└── logs/                 # Development logs
```

## Package Scripts

### Root Level
```bash
# Install all dependencies
npm run install:all

# Start all development servers
npm run dev

# Build all projects
npm run build

# Run all tests
npm run test

# Lint all code
npm run lint

# Format all code
npm run format
```

### React App
```bash
cd react-app
npm run dev          # Start dev server
npm run build        # Production build
npm run test         # Run tests
npm run type-check   # TypeScript checking
```

### Vue Components
```bash
cd vue-components
npm run dev          # Start dev server
npm run build        # Build library
npm run test         # Run tests
npm run docs         # Generate docs
```

### Storybook
```bash
cd storybook
npm run dev          # Start Storybook
npm run build        # Build static site
npm run test         # Visual regression tests
```

## Development Features

### 1. TypeScript Integration
- Strict type checking across all projects
- Shared type definitions in `shared/types/`
- Auto-import and IntelliSense support
- Build-time type validation

### 2. Modern Build Tools
- **Vite**: Fast dev server and bundling
- **esbuild**: Super-fast TypeScript transpilation
- **Rollup**: Optimized production builds
- **PostCSS**: Advanced CSS processing

### 3. Testing Ecosystem
- **Jest**: Unit and integration testing
- **Testing Library**: Component testing utilities
- **Cypress**: End-to-end testing
- **Chromatic**: Visual regression testing

### 4. Code Quality Tools
- **ESLint**: Customizable linting rules
- **Prettier**: Consistent code formatting
- **Husky**: Git hooks for quality gates
- **lint-staged**: Run linters on staged files

### 5. Component Development
- **Storybook**: Interactive component explorer
- **Vue Devtools**: Vue-specific debugging
- **React DevTools**: React debugging support
- **Hot Module Replacement**: Preserve state during updates

## Configuration Files

### ESLint (`tools/eslint/.eslintrc.js`)
```javascript
module.exports = {
  extends: [
    '@typescript-eslint/recommended',
    'plugin:react/recommended',
    'plugin:vue/vue3-recommended'
  ],
  rules: {
    'react/react-in-jsx-scope': 'off',
    'vue/multi-word-component-names': 'warn'
  }
}
```

### Prettier (`tools/prettier/.prettierrc`)
```json
{
  "semi": true,
  "singleQuote": true,
  "tabWidth": 2,
  "trailingComma": "es5"
}
```

### TypeScript (`shared/tsconfig.json`)
```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "jsx": "react-jsx"
  }
}
```

## Deployment

### Development
```bash
# Start all services
make run

# Individual services
npm run dev:react
npm run dev:vue
npm run dev:storybook
```

### Production
```bash
# Build all projects
npm run build:all

# Deploy static files
npm run deploy

# Docker deployment
docker-compose up -d
```

### CI/CD Integration
```yaml
# .github/workflows/frontend.yml
name: Frontend CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-node@v3
        with:
          node-version: '18'
      - run: npm ci
      - run: npm run lint
      - run: npm run test
      - run: npm run build
```

## Advanced Features

### 1. Micro-Frontend Architecture
- Independent deployments
- Shared component library
- Module federation support
- Cross-app communication

### 2. Performance Optimization
- Code splitting and lazy loading
- Bundle analysis and optimization
- Image optimization pipeline
- Service worker integration

### 3. Internationalization
- Multi-language support
- Locale-specific builds
- Translation management
- RTL language support

### 4. Accessibility
- ARIA compliance checking
- Color contrast validation
- Keyboard navigation testing
- Screen reader testing

## Troubleshooting

**Port conflicts:**
```bash
# Kill processes on common ports
lsof -ti:3000,3001,6006 | xargs kill -9
```

**Module resolution issues:**
- Clear node_modules: `rm -rf node_modules && npm install`
- Clear build cache: `npm run clean`
- Check workspace dependencies

**TypeScript errors:**
- Run type checking: `npm run type-check`
- Update type definitions: `npm update @types/*`
- Clear TypeScript cache: `npx tsc --build --clean`

**Hot reload not working:**
- Check file watching limits: `echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf`
- Restart development servers
- Check network configuration

## Extensions

### Adding New Frameworks
```bash
# Add Angular workspace
ng new angular-app --routing --style=scss
cd angular-app && npm link ../shared

# Add Svelte component
npm create svelte@latest svelte-components
cd svelte-components && npm install
```

### Custom Build Tools
```javascript
// Add Webpack configuration
const path = require('path');
module.exports = {
  entry: './src/index.ts',
  output: {
    path: path.resolve(__dirname, 'dist'),
    filename: 'bundle.js'
  },
  module: {
    rules: [
      {
        test: /\.tsx?$/,
        use: 'ts-loader'
      }
    ]
  }
};
```

## Best Practices

1. **Component Design**: Keep components small and focused
2. **State Management**: Use appropriate state solutions for scale
3. **Testing Strategy**: Test behavior, not implementation
4. **Performance**: Measure and optimize based on real usage
5. **Accessibility**: Design with accessibility from the start
6. **Documentation**: Keep Storybook stories up to date

## Next Steps

- Explore server-side rendering (Next.js/Nuxt.js)
- Add GraphQL integration
- Implement advanced testing strategies
- Set up performance monitoring
- Create design system documentation