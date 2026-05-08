/* einar embedded SDK v1
 * --------------------------------------------------------------
 * Tiny client que devs incluyen en su web cuando la van a embeber
 * en un workspace einar. Resuelve el handshake postMessage con el
 * shell, mantiene un token fresco, y expone una API mínima.
 *
 * Uso:
 *   <script src="https://einar.exe.xyz/sdk/v1.js"></script>
 *   <script>
 *     const e = einar.create({ appId: 'mi-app' });
 *     await e.ready();
 *     const token = e.getToken();
 *     const user  = e.getUser();
 *     fetch('/mi-api', { headers: { Authorization: 'Bearer ' + token }})
 *   </script>
 *
 * Protocolo postMessage (versión 1):
 *   iframe → parent: { type: 'einar:ready',   version: 1, appId }
 *   parent → iframe: { type: 'einar:auth',    version: 1, token, expiresAt, user }
 *   iframe → parent: { type: 'einar:refresh', version: 1 }
 *   iframe → parent: { type: 'einar:logout',  version: 1 }
 *   parent → iframe: { type: 'einar:session-ended', version: 1 }
 *
 * Importante: el iframe NUNCA debe asumir que está embebido. Si
 * window.parent === window, el SDK queda en estado pending y los
 * getters devuelven null. Esto permite que la misma app web
 * funcione standalone y embebida sin código condicional pesado.
 */
(function (global) {
  'use strict';

  const VERSION = 1;
  const SHELL_ORIGIN_PROD = 'https://einar.exe.xyz';

  function create(opts) {
    opts = opts || {};
    const appId = opts.appId || '';
    // El origin esperado del shell. Si te embebés en otro entorno
    // (ej. dev local), pasalo explícito en `opts.shellOrigin`.
    const shellOrigin = opts.shellOrigin || SHELL_ORIGIN_PROD;

    let token = null;
    let expiresAt = 0;
    let user = null;
    let issuer = null;
    let jwksUri = null;
    const authListeners = [];
    const sessionEndedListeners = [];

    let readyResolve;
    const readyPromise = new Promise((r) => { readyResolve = r; });

    if (window.parent === window) {
      // Standalone: no estamos en un iframe. La promise nunca resuelve;
      // el dev decide qué hacer con eso (mostrar mensaje, etc.).
      console.warn('[einar] not embedded; running standalone');
      return makeApi();
    }

    function onMessage(ev) {
      // Origin check estricto: solo aceptamos mensajes del shell.
      if (ev.origin !== shellOrigin) return;

      const msg = ev.data;
      if (!msg || typeof msg !== 'object') return;

      switch (msg.type) {
        case 'einar:auth':
          token = msg.token;
          expiresAt = msg.expiresAt;
          user = msg.user;
          issuer = msg.issuer || null;
          jwksUri = msg.jwksUri || null;
          authListeners.forEach((cb) => { try { cb({ token, user, expiresAt, issuer, jwksUri }); } catch (_) {} });
          if (readyResolve) { readyResolve(); readyResolve = null; }
          break;
        case 'einar:session-ended':
          token = null; expiresAt = 0; user = null;
          sessionEndedListeners.forEach((cb) => { try { cb(); } catch (_) {} });
          break;
      }
    }
    window.addEventListener('message', onMessage);

    // Anunciar que estamos listos. El shell debería responder con
    // 'einar:auth' enseguida.
    window.parent.postMessage(
      { type: 'einar:ready', version: VERSION, appId: appId },
      shellOrigin,
    );

    function makeApi() {
      return {
        ready: function () { return readyPromise; },
        getToken: function () {
          // Hint: si está cerca de expirar, pedimos refresh sin bloquear.
          if (token && Date.now() / 1000 > expiresAt - 30) {
            try {
              window.parent.postMessage(
                { type: 'einar:refresh', version: VERSION },
                shellOrigin,
              );
            } catch (_) {}
          }
          return token;
        },
        getUser: function () { return user; },
        getExpiresAt: function () { return expiresAt; },

        // Metadata para que el backend del dev sepa dónde validar:
        //   issuer  → claim 'iss' del JWT
        //   jwksUri → endpoint público con la public key (RS256)
        // Discovery doc completo: <issuer>/.well-known/einar/openid-configuration
        getIssuer:  function () { return issuer; },
        getJwksUri: function () { return jwksUri; },

        onAuth: function (cb) {
          authListeners.push(cb);
          return function () {
            const i = authListeners.indexOf(cb);
            if (i >= 0) authListeners.splice(i, 1);
          };
        },
        onSessionEnded: function (cb) {
          sessionEndedListeners.push(cb);
          return function () {
            const i = sessionEndedListeners.indexOf(cb);
            if (i >= 0) sessionEndedListeners.splice(i, 1);
          };
        },

        logout: function () {
          window.parent.postMessage(
            { type: 'einar:logout', version: VERSION },
            shellOrigin,
          );
        },

        // Helper: fetch con Authorization Bearer auto-inyectado.
        // Si el token está vencido, espera al refresh antes de mandar.
        fetch: async function (input, init) {
          const t = token;
          if (!t) throw new Error('einar: no token yet — await einar.ready()');
          const headers = new Headers((init && init.headers) || {});
          if (!headers.has('Authorization')) {
            headers.set('Authorization', 'Bearer ' + t);
          }
          return fetch(input, Object.assign({}, init, { headers: headers }));
        },
      };
    }

    return makeApi();
  }

  // Expone el namespace global. Si querés ESM en el futuro, exportá
  // create como default desde un .mjs separado.
  global.einar = { create: create, version: VERSION };
})(window);
