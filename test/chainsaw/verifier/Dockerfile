FROM amazon/aws-cli:latest
# aws-cli base is amazonlinux; add jq for BucketInfo parsing and curl for the
# admin API (curl in amazonlinux already supports --aws-sigv4).
RUN yum install -y jq && yum clean all
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
COPY admin.sh /usr/local/bin/admin.sh
RUN chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/admin.sh
# Default entrypoint is user-mode; admin Jobs override to /usr/local/bin/admin.sh.
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
