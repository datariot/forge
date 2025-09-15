# Forge Documentation Site

This directory contains the GitHub Pages documentation site for the Forge framework.

## Local Development

### Prerequisites

1. **Ruby** (version 3.1 or higher)
2. **Bundler** gem

### Setup

```bash
cd docs
bundle install
```

### Running Locally

```bash
cd docs
bundle exec jekyll serve
```

The site will be available at `http://localhost:4000/forge/`

### Building for Production

```bash
cd docs
bundle exec jekyll build
```

## Site Structure

```
docs/
├── _config.yml          # Jekyll configuration
├── index.md             # Homepage
├── getting-started.md   # Getting started guide
├── bundles.md          # Bundle overview
├── examples.md         # Example services
├── api-reference.md    # API documentation
├── Gemfile             # Ruby dependencies
└── _layouts/           # Page layouts (optional)
```

## Deployment

The site is automatically deployed to GitHub Pages when:
- Changes are pushed to the `main` branch in the `docs/` directory
- The workflow is manually triggered

GitHub Pages URL: `https://datariot.github.io/forge/`

## Content Guidelines

### Documentation Standards

1. **Clear Navigation** - Logical information hierarchy
2. **Code Examples** - Working code snippets for all features
3. **Security Focus** - Highlight security best practices
4. **Production Ready** - Include deployment and configuration guidance
5. **Developer Experience** - Easy to follow tutorials and references

### Writing Style

- **Concise** - Get to the point quickly
- **Practical** - Focus on real-world usage
- **Complete** - Include all necessary information
- **Updated** - Keep documentation in sync with code

## Contributing

To update the documentation:

1. Edit files in the `docs/` directory
2. Test locally with `bundle exec jekyll serve`
3. Commit and push to the `main` branch
4. GitHub Actions will automatically deploy the changes