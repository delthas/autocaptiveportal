.POSIX:
.SUFFIXES:

GO       ?= go
RM       ?= rm
INSTALL  ?= install
GIT      ?= git
GOFLAGS  ?=
PREFIX   ?= /usr/local
BINDIR   ?= bin
NMDIR    ?= /etc/NetworkManager/dispatcher.d

ifeq (0, $(shell $(GIT) rev-parse HEAD >/dev/null 2>&1; echo $$?))
export SOURCE_DATE_EPOCH ?= $(shell $(GIT) log -1 --pretty=%ct)
endif

all: autocaptiveportal

autocaptiveportal:
	$(GO) build $(GOFLAGS) .

clean:
	$(RM) -f autocaptiveportal

install:
	$(INSTALL) -d $(DESTDIR)$(PREFIX)/$(BINDIR)
	$(INSTALL) -m755 autocaptiveportal $(DESTDIR)$(PREFIX)/$(BINDIR)/autocaptiveportal
	$(INSTALL) -d $(DESTDIR)$(NMDIR)/no-wait.d
	ln -sf $(PREFIX)/$(BINDIR)/autocaptiveportal $(DESTDIR)$(NMDIR)/no-wait.d/autocaptiveportal

uninstall:
	$(RM) -f $(DESTDIR)$(PREFIX)/$(BINDIR)/autocaptiveportal
	$(RM) -f $(DESTDIR)$(NMDIR)/no-wait.d/autocaptiveportal

.PHONY: all autocaptiveportal clean install uninstall
