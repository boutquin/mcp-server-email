.PHONY: test test-integration lint docker-build

test:
	go test -race -count=1 ./...

test-integration:
	docker run -d --name greenmail -p 3465:3465 -p 3993:3993 \
		-e "GREENMAIL_OPTS=-Dgreenmail.setup.test.all -Dgreenmail.users=test:password@example.com -Dgreenmail.hostname=0.0.0.0" \
		greenmail/standalone:2.1.0
	sleep 3
	TEST_IMAP_HOST=localhost TEST_IMAP_PORT=3993 TEST_SMTP_HOST=localhost \
		TEST_SMTP_PORT=3465 TEST_EMAIL=test@example.com TEST_PASSWORD=password \
		go test -tags=integration -race ./... ; \
	EXIT_CODE=$$? ; \
	docker stop greenmail && docker rm greenmail ; \
	exit $$EXIT_CODE

lint:
	golangci-lint run ./...

docker-build:
	docker build -t mcp-server-email .
