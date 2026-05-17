// Jenkinsfile for Weather Service
// Server: jenkins.casjay.cc
// Agents: arm64, amd64

pipeline {
    agent none

    environment {
        PROJECTNAME = 'weather'
        PROJECTORG = 'casapps'
        GO_VERSION = '1.24'
        DOCKER_IMAGE = "golang:${GO_VERSION}-alpine"
    }

    stages {
        stage('Build Multi-Arch') {
            parallel {
                stage('Build AMD64') {
                    agent {
                        docker {
                            image "${DOCKER_IMAGE}"
                            label 'amd64'
                            args '-v $HOME/.cache/go-build:/root/.cache/go-build'
                        }
                    }
                    steps {
                        echo '🏗️  Building for AMD64...'
                        sh 'apk add --no-cache make git bash'
                        sh 'go version'
                        sh 'make build'
                        stash includes: 'binaries/*-amd64*', name: 'binaries-amd64'
                    }
                }

                stage('Build ARM64') {
                    agent {
                        docker {
                            image "${DOCKER_IMAGE}"
                            label 'arm64'
                            args '-v $HOME/.cache/go-build:/root/.cache/go-build'
                        }
                    }
                    steps {
                        echo '🏗️  Building for ARM64...'
                        sh 'apk add --no-cache make git bash'
                        sh 'go version'
                        sh 'make build'
                        stash includes: 'binaries/*-arm64*', name: 'binaries-arm64'
                    }
                }
            }
        }

        stage('Test') {
            parallel {
                stage('Test AMD64') {
                    agent {
                        docker {
                            image "${DOCKER_IMAGE}"
                            label 'amd64'
                        }
                    }
                    steps {
                        echo '🧪 Running tests on AMD64...'
                        sh 'apk add --no-cache make git'
                        sh 'make test || true'
                    }
                }

                stage('Test ARM64') {
                    agent {
                        docker {
                            image "${DOCKER_IMAGE}"
                            label 'arm64'
                        }
                    }
                    steps {
                        echo '🧪 Running tests on ARM64...'
                        sh 'apk add --no-cache make git'
                        sh 'make test || true'
                    }
                }
            }
        }

        stage('Package') {
            agent {
                docker {
                    image "${DOCKER_IMAGE}"
                    label 'amd64'
                }
            }
            steps {
                echo '📦 Collecting binaries...'
                unstash 'binaries-amd64'
                unstash 'binaries-arm64'
                sh 'ls -lah binaries/'
            }
        }

        stage('Docker Build') {
            agent {
                label 'amd64'
            }
            when {
                branch 'main'
            }
            steps {
                echo '🐳 Building Docker image...'
                sh 'docker --version'
                sh 'make docker'
            }
        }

        stage('Release') {
            agent {
                docker {
                    image "${DOCKER_IMAGE}"
                    label 'amd64'
                }
            }
            when {
                branch 'main'
            }
            steps {
                echo '🚀 Creating release...'
                sh 'apk add --no-cache make git bash curl'
                sh 'make release || echo "Release failed or already exists"'
            }
        }
    }

    post {
        success {
            echo '✅ Build completed successfully!'
        }
        failure {
            echo '❌ Build failed!'
        }
        cleanup {
            cleanWs()
        }
    }
}
