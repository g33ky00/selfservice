---
name: expose_internal_service
description: "Instancier un accès temporaire à un service interne via tunnel Cloudflare + Access OTP, avec TTL et destruction automatique."
author: Hermes Agent
license: MIT
version: "2026-07-16"
---

# Expose Internal Service — Tunnel CF éphémère générique

## Use this skill when
- L’utilisateur demande un accès à la demande vers une machine/service interne depuis l’extérieur
- Requête contenant : "accès à la demande", "tunnel temporaire", "exposer ponctuellement", "expose", "lien temporaire", "self-service", "ss.coresynq.cc"
- Besoin exposant un service/port/cible variables, non limité à une liste figée, déclenché par simple demande utilisateur
- Sortie attendue : lien cliquable public temporaire vers `ss.coresynq.cc` utilisable dans un navigateur ou client SSH

## Architecture validée V2 (adopter par défaut)
- Surface publique statique : `ss.coresynq.cc` pointant vers un tunnel Cloudflare permanent unique `ss`
- Pas de création/suppression DNS par session : le CNAME de `ss.coresynq.cc` est stable
- L’ingress de session est manipulée via la config du tunnel `ss` :
  - session active : routage vers `<scheme>://<target_ip>:<port>`
  - session expirée/fermée : `http_status:404` pour `/`, avec chemin de session dédié pour les services websocket/TCP comme Gotty
- Auth : token aléatoire 32+ caractères dans le chemin URL `ss.coresynq.cc/<token>`, couplé à un TTL 15 minutes strict ; Cloudflare Access OTP refusé par défaut pour cause de friction.
- GC : timer systemd utilisateur transient via `systemd-run --user --property=RuntimeMaxSec=<ttl>`.
- Fichier d’état sessions dans le skill : `chmod 600` obligatoire.

## Entrée attendue (JSON)
```json
{
  "service": "ssh",
  "port": 22,
  "target": "pytheas",
  "target_ip": "192.168.2.40",
  "ttl": 900,
  "scheme": "tcp"
}
```
- `service` : logique métier (`ssh`, `http`, `https`, `custom`…)
- `port` : port TCP cible interne
- `target` : nom logique interne, utilisé comme préfixe humain
- `target_ip` : IP privée du service interne ; peut varier à la demande
- `ttl` : secondes ; défaut `900` (15 min). Max autorisé `7200`. Au-delà, forcer `7200` + avertir.
- `scheme` : `tcp` pour SSH/ports bruts, `http`/`https` pour applicatifs web

## Entrée alternative courte
Si l’utilisateur fournit uniquement un contexte naturel, Hermes extrait :
- `target_ip` depuis la demande si explicite, sinon depuis un mapping local si disponible
- `port` depuis la demande
- `service` déduit du port (`22` → `ssh`, `8006` → `http`, `443` → `https`, autre → `custom`)
- `scheme` déduit du service (`ssh` → `tcp`, autre → `http`)

## Prérequis système
- `cloudflared` installé et dans `PATH`
- `systemd-run --user` disponible
- Accès à l’API Cloudflare avec droits : Tunnel Edit, DNS Edit
- Environment Cloudflare chargé :
  `set -a; . /home/g33ky/.config/cloudflare/credentials.env; set +a`
- Le script manager doit rester dans `scripts/` du skill.

## Pièges validés en production
- Création tunnel : utiliser `POST .../cfd_tunnel` avec `{"name":"ss"}` seulement. Le champ `config_src` contenant `tunnel:` n’est pas accepté dans certains comptes (`unknown variant tunnel, expected local or cloudflare`). Configurer l’ingress ensuite via `PUT .../cfd_tunnel/<id>/configurations`.
- Endpoint config/ingress : utiliser uniquement `PUT /accounts/{account_id}/cfd_tunnel/{tunnel_id}/configurations`. Ne pas utiliser `/config` (404) ni `PATCH`. Payload wrapper obligatoire : `{"config":{"ingress":[...]}}`. Dernière règle : catch-all sans `hostname`/`path`, sinon Cloudflare retourne `1056 Bad Configuration`.
- Bash heredocs Python : ne pas attendre un `<<'PY'` quoted pour récupérer `ACCOUNT_ID`/`TUNNEL_ID` ; passer par `sys.argv` depuis le script bash plutôt que `os.environ[...]`.
- DNS CNAME : pour un tunnel actif, le CNAME doit être `proxied: true`. Si `proxied: false`, le trafic ne passe pas par la couche d’inspection Cloudflare et les règles d’ingress peuvent ne pas s’appliquer. Vérifier avec `zones/{zone_id}/dns_records` avant de suspecter une panne tunnel.
- Auth 400 sur config endpoint : un `PUT /configurations` qui retourne `Authentication failed (status: 400)` indique généralement un token sans scope tunnels/zero-trust, pas un problème global d’auth. `/user/tokens/verify` peut être trompeur.
- Gotty derrière CF Tunnel : ajouter `--ws-origin "<hostname>"` pour éviter le rejet WebSocket par gotty. Exemple : `gotty -w -r --ws-origin "ss.coresynq.cc" ssh user@host`.
- Validation réseau locale : un timeout `curl` depuis l’origine vers `ss.coresynq.cc` est souvent du hairpinning/NAT loopback, pas une preuve de panne. Ne pas perdre de temps dessus ; valider par l’API CF et par un test 4G extérieur.

## Domaine public
- Sous-domaine unique dédié : `ss.coresynq.cc`
- CNAME vers le tunnel permanent `ss` déjà provisionné ; ne pas le recréer/supprimer par session
- URL publique finale : `https://ss.coresynq.cc/<session_token>`

## Sortie stricte
Succès :
```json
{
  "status": "success",
  "public_url": "https://ss.coresynq.cc/<session_token>",
  "session_id": "<short-id>",
  "message": "Session temporaire établie.",
  "ttl_seconds": <ttl>,
  "service": "<service>",
  "target": "<target>",
  "port": <port>,
  "scheme": "<scheme>"
}
```
Échec :
```json
{
  "status": "error",
  "step_failed": "<etape>",
  "details": "<message>"
}
```

## Rôles et étapes du script manager
Fichier : `scripts/cf_tunnel_manager.py` dans le répertoire du skill.

| Action | Sortie JSON | Comportement |
|---|---|---|
| `provision` | URL + TTL | Activer ingress temporaire, lancer daemon + GC |
| `destroy` | `{"status":"destroyed"}` | Désactiver ingress, tuer processus |
| `cleanup` | `{"status":"ok","count":N}` | Purger états orphelins |

### Phase A — Activation ingress session
1. Générer `SESSION_ID` + `session_token` aléatoire 32+ caractères
2. Mettre à jour la config du tunnel permanent `ss` :
   - ingress session : `<scheme>://<target_ip>:<port>` pour chemin `/<session_token>` quand nécessaire, ou root temporaire si service web simple
   - fallback : `http_status:404`
3. Écrire `/tmp/selfservice_<SESSION_ID>.json` avec `SESSION_ID,session_token,service,port,target,target_ip,ttl,status`

### Phase B — Lancement service local
- SSH via navigateur : lancer `gotty` sur un port local éphémère, enregistrer son PID
- HTTP/HTTPS direct : pas de gotty nécessaire
- Commande :
  `systemd-run --user --unit=selfservice-<SESSION_ID> --property=RuntimeMaxSec=<ttl> <cmd_local>`

### Phase C — Planificateur de destruction
- Créer un timer systemd utilisateur éphémère `selfservice-cleanup-<SESSION_ID>.timer` exécutant `scripts/cf_tunnel_manager.py --action destroy --session-id <SESSION_ID>`
- Le chemin du script manager doit être résolu dynamiquement depuis le répertoire du skill, pas codé en dur.
- Fallback unique autorisé : `at now + <ttl> seconds` si `systemd-run --user` échoue, sinon erreur.

### Phase D — Destruction
- Lit `/tmp/selfservice_<SESSION_ID>.json`
- Réinitialise l’ingress du tunnel `ss` vers `http_status:404`
- Tue le process local/gotty associé
- Supprime le fichier d’état

### Nettoyage des orphelins
- Au démarrage de tout `provision` : examiner `/tmp/selfservice_*.json`
- Si `ttl` écoulée ou fichier ancien de plus de `max(ttl,3600)`, tenter destroy
- Renvoyer `cleanup` avec nombre d’entrées nettoyées

## Gestion d’erreurs
- Chaque appel API Cloudflare vérifie HTTP 200 et `success:true`
- Toute erreur 4xx/5xx retourne `step_failed` parmi :
  `TUNNEL_CONFIG`, `DNS_UPDATE`, `LOCAL_RUNNER`, `SCHEDULE`, `DESTROY`
- Aucune impression hors JSON sur stdout ; logs autorisés sur stderr uniquement si debug activé par flag non contractuel

## Sécurité
- Secrets depuis `/home/g33ky/.config/cloudflare/credentials.env` uniquement
- Ne jamais logger `CLOUDFLARE_API_TOKEN`
- Vérifier que `/home/g33ky/.config/cloudflare/credentials.env` a permissions `600`
- Ne pas exposer les jetons `TUNNEL_TOKEN` dans la réponse
- `chmod 600` obligatoire sur `/tmp/selfservice_*.json` car contient tokens actifs
- Authentifier le demandeur avec une règle locale avant provision :
  - canal/chat Telegram idoine attendu pour les demandes
  - en environnement non Telegram : vérifier la source avant tout appel API
- Détruire la session à la demande de l’utilisateur ou après TTL ; ne pas laisser persister
- L’accès doit être considéré comme public non authentifié : l’entropie du token + TTL est le seul contrôle. Aucun chemin doit être exposé sans token.

## Limites
- Un seul service actif à la fois sur `ss.coresynq.cc` sauf rotation explicite
- Targets/services/ports libres ; pas de mapping fixe obligatoire
- `ttl` max autorisé : `7200` ; au-delà forcer `7200` et avertir
- L’ingress de secours `http_status:404` doit rester la règle par défaut quand aucune session n’est active.
- The permanent tunnel `ss` and its CNAME are owned by this skill. Other code or operators must not modify `ss.coresynq.cc` unless explicitly requested in-session.
- **NEVER touch protected domains declared by the user outside the requested scope**, e.g. `thaecreations.com`, unless the user explicitly authorizes a change in the same operative request.

## Références internes
- `references/architecture-v2.md` : décision V2, justification sécurité, comparaison V1.
- `references/environment_constraints.md` : contraintes découvertes dans la session, notamment absence de `at` et fallback timer systemd, chemin du script manager, mappings scheme/port par target, zone `coresynq.cc`.
- `references/cloudflare_tunnel_api_pitfalls.md` : format création/config tunnel, erreurs JSON observées, ordre create-then-patch.
- `references/selfservice_pattern.md` : pattern générique d’auto-inscription utilisateur pour exposition à la demande via `ss.coresynq.cc`.
