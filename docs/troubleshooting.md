Here is the content for `troubleshooting.md`:

```markdown
# Troubleshooting

## Common Errors

### Git tree is dirty
If you encounter the error that your Git tree is dirty, ensure you have committed all changes and cleaned up the stash. You can disable this behavior with the following command:

```bash
set -Ux NIX_GIT_CHECKS false
```

### Permission Issues
If you encounter permission issues while
