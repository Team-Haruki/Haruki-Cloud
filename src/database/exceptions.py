class ClientNotInitialized(Exception):
    def __init__(self, message: str = "Haruki DB API Client is not initialized."):
        self.message = message
        super().__init__(self.message)


class InvalidDatabaseType(Exception):
    def __init__(self, message: str = "You have entered an invalid database type."):
        self.message = message
        super().__init__(self.message)
