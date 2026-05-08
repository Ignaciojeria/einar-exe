{
  "organizations": [
    {
      "owner": "admin",
      "name": "einar",
      "displayName": "Einar",
      "websiteUrl": "${APP_PUBLIC_URL}",
      "defaultApplication": "einar-app",
      "defaultAvatar": "",
      "tags": [],
      "languages": ["en", "es", "zh", "fr", "de", "ja", "ko", "ru", "pt"],
      "countryCodes": ["US", "ES", "CL", "MX", "AR", "BR"],
      "masterPassword": "",
      "enableSoftDeletion": false,
      "isProfilePublic": false,
      "accountItems": [
        {"name": "Organization", "visible": true, "viewRule": "Public", "modifyRule": "Admin"},
        {"name": "ID",           "visible": true, "viewRule": "Public", "modifyRule": "Immutable"},
        {"name": "Name",         "visible": true, "viewRule": "Public", "modifyRule": "Admin"},
        {"name": "Display name", "visible": true, "viewRule": "Public", "modifyRule": "Self"},
        {"name": "Avatar",       "visible": true, "viewRule": "Public", "modifyRule": "Self"},
        {"name": "Email",        "visible": true, "viewRule": "Public", "modifyRule": "Self"},
        {"name": "Phone",        "visible": true, "viewRule": "Public", "modifyRule": "Self"},
        {"name": "Password",     "visible": true, "viewRule": "Self",   "modifyRule": "Self"},
        {"name": "Is admin",     "visible": true, "viewRule": "Admin",  "modifyRule": "Admin"},
        {"name": "Is forbidden", "visible": true, "viewRule": "Admin",  "modifyRule": "Admin"},
        {"name": "Is deleted",   "visible": true, "viewRule": "Admin",  "modifyRule": "Admin"}
      ]
    }
  ],
  "providers": [
    {
      "owner": "admin",
      "name": "provider_google_einar",
      "displayName": "Google",
      "category": "OAuth",
      "type": "Google",
      "clientId": "${GOOGLE_CLIENT_ID}",
      "clientSecret": "${GOOGLE_CLIENT_SECRET}",
      "scopes": "profile+email",
      "providerUrl": ""
    }
  ],
  "applications": [
    {
      "owner": "admin",
      "name": "einar-app",
      "displayName": "Einar App",
      "logo": "https://cdn.casbin.org/img/casdoor-logo_1185x256.png",
      "homepageUrl": "${APP_PUBLIC_URL}",
      "organization": "einar",
      "cert": "cert-built-in",
      "enablePassword": false,
      "enableSignUp": true,
      "enableSigninSession": true,
      "enableAutoSignin": false,
      "enableCodeSignin": false,
      "providers": [
        {
          "name": "provider_google_einar",
          "canSignUp": true,
          "canSignIn": true,
          "canUnlink": true,
          "prompted": false,
          "alertType": "None"
        }
      ],
      "signupItems": [
        {"name": "ID",               "visible": false, "required": true, "prompted": false, "rule": "Random"},
        {"name": "Username",         "visible": true,  "required": true, "prompted": false, "rule": "None"},
        {"name": "Display name",     "visible": true,  "required": true, "prompted": false, "rule": "None"},
        {"name": "Password",         "visible": true,  "required": true, "prompted": false, "rule": "None"},
        {"name": "Confirm password", "visible": true,  "required": true, "prompted": false, "rule": "None"},
        {"name": "Email",            "visible": true,  "required": true, "prompted": false, "rule": "Normal"},
        {"name": "Phone",            "visible": true,  "required": true, "prompted": false, "rule": "None"},
        {"name": "Agreement",        "visible": true,  "required": true, "prompted": false, "rule": "None"}
      ],
      "redirectUris": ["${APP_PUBLIC_URL}/auth/callback"],
      "tokenFormat": "JWT",
      "expireInHours": 168,
      "refreshExpireInHours": 168,
      "signinMethods": [
        {"name": "Password",          "displayName": "Password",          "rule": "None"},
        {"name": "Verification code", "displayName": "Verification code", "rule": "None"},
        {"name": "WebAuthn",          "displayName": "WebAuthn",          "rule": "None"},
        {"name": "LDAP",              "displayName": "LDAP",              "rule": "None"}
      ],
      "grantTypes": [
        "authorization_code",
        "refresh_token"
      ],
      "tags": [],
      "clientId": "${CASDOOR_CLIENT_ID}",
      "clientSecret": "${CASDOOR_CLIENT_SECRET}"
    }
  ]
}
