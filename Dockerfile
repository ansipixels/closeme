FROM scratch
COPY closeme /usr/bin/closeme
ENV HOME=/home/user
ENTRYPOINT ["/usr/bin/closeme"]
