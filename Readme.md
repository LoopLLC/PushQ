Pushq 
=====

Pushq is a thin wrapper over Google's push queues.  POST a payload and a URL, and this service will POST the payload to the URL with retries if the URL does not return a 200 response.

In addition to the simple REST API, this project has an administrative console for creating API Keys and viewing statistics.

REST API
--------

- /enq  POST

Enqueue a task.

    {
        "url":"http://localhost:8080/test",
        "delaySeconds":1,
        "payload":"ABC",
        "queueName":"default",
        "headers":null,
        "timeoutSeconds":5
    }

