# SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
# SPDX-License-Identifier: MIT
FROM    golang:alpine

RUN     apk --update --no-cache add \
        gst-plugins-base-dev \
        gstreamer-dev \
        build-base \
        git

WORKDIR /rtwatch

ARG     REPO=https://github.com/pion/rtwatch.git
ARG     BRANCH=master

RUN     echo -e "GIT Repo: $REPO\nGIT Branch: $BRANCH"

RUN     git clone https://github.com/pion/rtwatch.git --progress --verbose --branch $BRANCH /rtwatch

RUN     go install

FROM    alpine:latest

RUN     apk --update --no-cache add \
        gst-plugins-good \
        gst-plugins-ugly \
        gst-plugins-bad \
        gstreamer

COPY    --from=0 /go/bin/rtwatch /usr/local/bin/rtwatch

ENTRYPOINT      [ "/usr/local/bin/rtwatch" ]
CMD             [ "-container-path", "https://ia800207.us.archive.org/15/items/BigBuckBunny_124/Content/big_buck_bunny_720p_surround.mp4" ]
