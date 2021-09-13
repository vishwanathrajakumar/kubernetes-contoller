FROM alpine
COPY ./kubernetes-controller /usr/local/bin/kubernetes-controller
ENTRYPOINT ["/usr/local/bin/kubernetes-controller"]