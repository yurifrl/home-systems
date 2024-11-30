```bash
kill-node.sh
sudo systemctl restart secret-loader & sudo journalctl -u secret-loader -f 
sudo systemctl restart argo-setup & sudo journalctl -u argo-setup -f 
```
