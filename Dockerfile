FROM scratch
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/vault-plugin-secrets-litellm /vault-plugin-secrets-litellm
ENTRYPOINT ["/vault-plugin-secrets-litellm"]
